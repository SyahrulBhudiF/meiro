//go:build darwin

package systemmedia

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/go-macos/objc"
)

func init() { runtime.LockOSThread() }

// Cocoa and the artwork completion queue need the process's main thread.
func TestMain(m *testing.M) {
	if err := objc.Load(objc.Foundation, objc.AppKit); err != nil {
		panic(err)
	}
	result := make(chan int, 1)
	go func() { result <- m.Run() }()
	for {
		select {
		case code := <-result:
			os.Exit(code)
		default:
			objc.AutoreleasePool(func() {
				until := objc.ClassID("NSDate").Send(objc.Sel("dateWithTimeIntervalSinceNow:"), 0.01)
				objc.ClassID("NSRunLoop").Send(objc.Sel("currentRunLoop")).Send(objc.Sel("runUntilDate:"), until)
			})
			time.Sleep(time.Millisecond)
		}
	}
}

func onMediaMain(t *testing.T, work func()) {
	t.Helper()
	done := make(chan struct{})
	objc.DispatchMain(func() {
		defer close(done)
		objc.AutoreleasePool(work)
	})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("macOS main queue did not run")
	}
}

func newTestMacSession(t *testing.T) Session {
	t.Helper()
	var session Session
	var err error
	onMediaMain(t, func() { session, err = New(Controls{}) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		onMediaMain(t, func() { err = session.Close() })
		if err != nil {
			t.Error(err)
		}
	})
	return session
}

func mediaInfo() objc.ID {
	return objc.ClassID("MPNowPlayingInfoCenter").Send(objc.Sel("defaultCenter")).Send(objc.Sel("nowPlayingInfo"))
}

func mediaNumber(key string) float64 {
	value := mediaInfo().Send(objc.Sel("objectForKey:"), objc.NSString(key))
	return objc.Send[float64](value, objc.Sel("doubleValue"))
}

func TestNowPlayingProgressAndExplicitChanges(t *testing.T) {
	session := newTestMacSession(t)
	state := State{VideoID: "song", Title: "A song", Artist: "An artist", Position: 17 * time.Second, Duration: 3 * time.Minute, Status: Playing, CanSeek: true}
	onMediaMain(t, func() { session.Update(state) })
	start := time.Now()
	time.Sleep(1100 * time.Millisecond)
	state.Position += time.Since(start)
	onMediaMain(t, func() { session.Update(state) })
	var elapsed, rate float64
	onMediaMain(t, func() { elapsed, rate = mediaNumber("elapsedPlaybackTime"), mediaNumber("playbackRate") })
	if elapsed != 17 || rate != 1 {
		t.Fatalf("normal progress republished metadata: elapsed=%v rate=%v; want 17, 1", elapsed, rate)
	}

	for _, status := range []Status{Playing, Paused, Playing} {
		state.Position += 100 * time.Millisecond
		state.SeekVersion++
		state.Status = status
		onMediaMain(t, func() { session.Update(state) })
		onMediaMain(t, func() { elapsed, rate = mediaNumber("elapsedPlaybackTime"), mediaNumber("playbackRate") })
		wantRate := float64(1)
		if status == Paused {
			wantRate = 0
		}
		if elapsed != state.Position.Seconds() || rate != wantRate {
			t.Errorf("%s seek published elapsed=%v rate=%v; want %v, %v", status, elapsed, rate, state.Position.Seconds(), wantRate)
		}
	}
	onMediaMain(t, func() { session.Update(State{}) })
	var info objc.ID
	onMediaMain(t, func() { info = mediaInfo() })
	if info != 0 {
		t.Fatal("empty track retained Now Playing metadata")
	}
}

func TestNowPlayingArtworkAndFailures(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 64, 32))); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		status int
		body   []byte
		want   bool
	}{
		{"valid cover", http.StatusOK, data.Bytes(), true},
		{"HTTP failure", http.StatusNotFound, nil, false},
		{"invalid image", http.StatusOK, []byte("not an image"), false},
		{"truncated image", http.StatusOK, data.Bytes()[:10], false},
		{"oversized response", http.StatusOK, make([]byte, (8<<20)+1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			served := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write(test.body)
				served <- struct{}{}
			}))
			t.Cleanup(server.Close)
			session := newTestMacSession(t)
			state := State{VideoID: "song", Title: "A song", Status: Paused, ArtworkURL: server.URL}
			onMediaMain(t, func() { session.Update(state) })
			select {
			case <-served:
			case <-time.After(5 * time.Second):
				t.Fatal("artwork was not requested")
			}
			var artwork objc.ID
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				onMediaMain(t, func() { artwork = mediaInfo().Send(objc.Sel("objectForKey:"), objc.NSString("artwork")) })
				if artwork != 0 {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if (artwork != 0) != test.want {
				t.Fatalf("artwork present=%v, want %v", artwork != 0, test.want)
			}
			var title string
			onMediaMain(t, func() {
				title = objc.Stringify(mediaInfo().Send(objc.Sel("objectForKey:"), objc.NSString("title")))
			})
			if title != "A song" {
				t.Errorf("artwork request changed title to %q", title)
			}
			if test.want {
				var size objc.NSSize
				onMediaMain(t, func() {
					image := artwork.Send(objc.Sel("imageAtSize:"), objc.NSSize{Width: 32, Height: 16})
					size = objc.Send[objc.NSSize](image, objc.Sel("size"))
				})
				if size.Width != 32 || size.Height != 16 {
					t.Errorf("requested artwork size = %+v, want 32x16", size)
				}
			}
		})
	}
}

func TestNowPlayingCancelsObsoleteArtwork(t *testing.T) {
	for _, closeSession := range []bool{false, true} {
		name := "track switch"
		if closeSession {
			name = "shutdown"
		}
		t.Run(name, func(t *testing.T) {
			started, canceled := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-r.Context().Done()
				close(canceled)
			}))
			t.Cleanup(server.Close)
			session := newTestMacSession(t)
			onMediaMain(t, func() { session.Update(State{VideoID: "old", Title: "Old song", ArtworkURL: server.URL}) })
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("old artwork did not start")
			}
			var err error
			onMediaMain(t, func() {
				if closeSession {
					err = session.Close()
				} else {
					session.Update(State{VideoID: "new", Title: "New song"})
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Fatal("obsolete artwork request was not canceled")
			}
			var title string
			onMediaMain(t, func() {
				if closeSession {
					session.Update(State{VideoID: "late", Title: "Late song"})
				}
				title = objc.Stringify(mediaInfo().Send(objc.Sel("objectForKey:"), objc.NSString("title")))
			})
			if (!closeSession && title != "New song") || (closeSession && title != "") {
				t.Errorf("obsolete request or closed update changed title to %q", title)
			}
		})
	}
}
