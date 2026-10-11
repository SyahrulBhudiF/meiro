//go:build linux

package systemmedia

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestTrackPathIsValidAndUniquePerVideo(t *testing.T) {
	first := trackPath(State{VideoID: "abc-123_def"})
	second := trackPath(State{VideoID: "different"})
	if !first.IsValid() || first == second {
		t.Errorf("track paths = %q and %q; want distinct valid object paths", first, second)
	}
	if got := trackPath(State{}); !got.IsValid() {
		t.Errorf("empty track path %q is invalid", got)
	}
}

func TestMPRISSessionPublishesStateAndForwardsCommands(t *testing.T) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("run under dbus-run-session to exercise the MPRIS D-Bus service")
	}

	commands := make(chan string, 8)
	seeks := make(chan time.Duration, 4)
	volumes := make(chan float64, 1)
	shuffles := make(chan bool, 1)
	repeats := make(chan string, 1)
	session, err := New(Controls{
		Play:     func() { commands <- "play" },
		Pause:    func() { commands <- "pause" },
		Toggle:   func() { commands <- "toggle" },
		Stop:     func() { commands <- "stop" },
		Next:     func() { commands <- "next" },
		Previous: func() { commands <- "previous" },
		Seek:     func(position time.Duration) { seeks <- position },
		SetVolume: func(volume float64) {
			volumes <- volume
		},
		SetShuffle: func(shuffle bool) { shuffles <- shuffle },
		SetRepeat:  func(status string) { repeats <- status },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Errorf("close MPRIS session: %v", err)
		}
	}()

	linux := session.(*linuxSession)
	state := State{
		VideoID: "video-1", Title: "A song", Artist: "An artist",
		ArtworkURL: "https://example.test/art.jpg", Duration: 3 * time.Minute,
		Position: 17 * time.Second, Status: Playing, Volume: 0.7,
		CanPlay: true, CanPause: true, CanNext: true, CanPrevious: true,
		CanSeek: true,
	}
	linux.Update(state)

	client, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	object := client.Object(linux.busName, objectPath)

	var metadata map[string]dbus.Variant
	metadataValue, err := object.GetProperty(playerInterface + ".Metadata")
	if err != nil {
		t.Fatal(err)
	}
	if err := metadataValue.Store(&metadata); err != nil {
		t.Fatal(err)
	}
	var title string
	if err := metadata["xesam:title"].Store(&title); err != nil || title != state.Title {
		t.Errorf("MPRIS title = %q, err %v; want %q", title, err, state.Title)
	}
	var status string
	statusValue, err := object.GetProperty(playerInterface + ".PlaybackStatus")
	if err != nil {
		t.Fatal(err)
	}
	if err := statusValue.Store(&status); err != nil || status != string(Playing) {
		t.Errorf("MPRIS status = %q, err %v; want %q", status, err, Playing)
	}
	var position int64
	positionValue, err := object.GetProperty(playerInterface + ".Position")
	if err != nil {
		t.Fatal(err)
	}
	if err := positionValue.Store(&position); err != nil || position != int64(state.Position/time.Microsecond) {
		t.Errorf("MPRIS position = %d, err %v; want %d", position, err, int64(state.Position/time.Microsecond))
	}
	state.Position, state.Status = 20*time.Second, Paused
	linux.Update(state)
	positionValue, err = object.GetProperty(playerInterface + ".Position")
	if err != nil {
		t.Fatal(err)
	}
	if err := positionValue.Store(&position); err != nil || position != int64(state.Position/time.Microsecond) {
		t.Errorf("paused MPRIS position = %d, err %v; want %d", position, err, int64(state.Position/time.Microsecond))
	}

	for _, method := range []struct{ name, want string }{
		{"Play", "play"},
		{"Pause", "pause"},
		{"PlayPause", "toggle"},
		{"Stop", "stop"},
		{"Next", "next"},
		{"Previous", "previous"},
	} {
		if err := object.Call(playerInterface+"."+method.name, 0).Err; err != nil {
			t.Fatalf("call %s: %v", method.name, err)
		}
		if got := <-commands; got != method.want {
			t.Errorf("%s command = %q, want %q", method.name, got, method.want)
		}
	}
	if err := object.Call(playerInterface+".Seek", 0, int64(5*time.Second/time.Microsecond)).Err; err != nil {
		t.Fatal(err)
	}
	if got := <-seeks; got != 5*time.Second {
		t.Errorf("Seek requested %v, want 5s", got)
	}
	trackID := metadata["mpris:trackid"].Value().(dbus.ObjectPath)
	if err := object.Call(playerInterface+".SetPosition", 0, trackID, int64(12*time.Second/time.Microsecond)).Err; err != nil {
		t.Fatal(err)
	}
	if got := <-seeks; got != 12*time.Second {
		t.Errorf("SetPosition requested %v, want 12s", got)
	}
	if err := object.Call("org.freedesktop.DBus.Properties.Set", 0, playerInterface, "Volume", dbus.MakeVariant(0.4)).Err; err != nil {
		t.Fatal(err)
	}
	if got := <-volumes; got != 0.4 {
		t.Errorf("Volume callback = %v, want 0.4", got)
	}
	if err := object.Call("org.freedesktop.DBus.Properties.Set", 0, playerInterface, "Shuffle", dbus.MakeVariant(true)).Err; err != nil {
		t.Fatal(err)
	}
	if got := <-shuffles; !got {
		t.Errorf("Shuffle callback = %v, want true", got)
	}
	if err := object.Call("org.freedesktop.DBus.Properties.Set", 0, playerInterface, "LoopStatus", dbus.MakeVariant("Playlist")).Err; err != nil {
		t.Fatal(err)
	}
	if got := <-repeats; got != "Playlist" {
		t.Errorf("LoopStatus callback = %q, want Playlist", got)
	}
	if err := object.Call("org.freedesktop.DBus.Properties.Set", 0, playerInterface, "Rate", dbus.MakeVariant(1.25)).Err; err == nil {
		t.Error("setting an unsupported playback rate should fail")
	}

	var xml string
	if err := object.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Store(&xml); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(xml, playerInterface) || !strings.Contains(xml, "SetPosition") {
		t.Errorf("MPRIS introspection is missing player methods: %s", xml)
	}
}

func TestMPRISChangesArePublishedWithoutDuplicates(t *testing.T) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("run under dbus-run-session to exercise MPRIS signals")
	}
	volumes := make(chan float64, 1)
	session, err := New(Controls{SetVolume: func(volume float64) { volumes <- volume }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	})
	state := State{
		VideoID: "first", Title: "First song", Artist: "An artist",
		ArtworkURL: "https://example.test/first.jpg", Duration: 3 * time.Minute,
		Position: 17 * time.Second, Status: Playing, Volume: 0.7, CanSeek: true,
	}
	session.Update(state)
	client, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.AddMatchSignal(dbus.WithMatchObjectPath(objectPath)); err != nil {
		t.Fatal(err)
	}
	signals := make(chan *dbus.Signal, 64)
	client.Signal(signals)
	object := client.Object(session.(*linuxSession).busName, objectPath)
	wantSignal := func(t *testing.T, member string) *dbus.Signal {
		t.Helper()
		select {
		case signal := <-signals:
			if signal.Name != member {
				t.Fatalf("signal = %s, want %s", signal.Name, member)
			}
			return signal
		case <-time.After(time.Second):
			t.Fatalf("missing %s signal", member)
			return nil
		}
	}
	wantQuiet := func(t *testing.T) {
		t.Helper()
		select {
		case signal := <-signals:
			t.Fatalf("unexpected signal: %+v", signal)
		case <-time.After(100 * time.Millisecond):
		}
	}
	wantProperty := func(t *testing.T, property string) dbus.Variant {
		t.Helper()
		signal := wantSignal(t, "org.freedesktop.DBus.Properties.PropertiesChanged")
		var iface string
		var changes map[string]dbus.Variant
		var invalidated []string
		if err := dbus.Store(signal.Body, &iface, &changes, &invalidated); err != nil {
			t.Fatal(err)
		}
		if iface != playerInterface || len(changes) != 1 || len(invalidated) != 0 {
			t.Fatalf("unexpected property changes: %+v", signal.Body)
		}
		value, ok := changes[property]
		if !ok {
			t.Fatalf("changes = %+v, want %s", changes, property)
		}
		return value
	}

	t.Run("unchanged snapshots and normal progress are quiet", func(t *testing.T) {
		for range 20 {
			session.Update(state)
		}
		state.Position += 250 * time.Millisecond
		session.Update(state)
		wantQuiet(t)
	})
	t.Run("capability changes publish only that property", func(t *testing.T) {
		for _, enabled := range []bool{true, false} {
			state.CanNext = enabled
			session.Update(state)
			if got := wantProperty(t, "CanGoNext").Value(); got != enabled {
				t.Errorf("CanGoNext = %v, want %v", got, enabled)
			}
			wantQuiet(t)
		}
	})
	t.Run("small playing seeks publish immediately", func(t *testing.T) {
		state.Position += 100 * time.Millisecond
		state.SeekVersion++
		session.Update(state)
		signal := wantSignal(t, playerInterface+".Seeked")
		var position int64
		if err := dbus.Store(signal.Body, &position); err != nil {
			t.Fatal(err)
		}
		if position != int64(state.Position/time.Microsecond) {
			t.Errorf("Seeked position = %d, want %v", position, state.Position)
		}
		value, err := object.GetProperty(playerInterface + ".Position")
		if err != nil || value.Value() != position {
			t.Errorf("readable position = %v, %v; want %d", value, err, position)
		}
		session.Update(state)
		wantQuiet(t)
	})
	t.Run("pause and resume preserve position", func(t *testing.T) {
		for _, status := range []Status{Paused, Playing} {
			state.Status = status
			session.Update(state)
			if got := wantProperty(t, "PlaybackStatus").Value(); got != string(status) {
				t.Errorf("status = %v, want %s", got, status)
			}
			wantQuiet(t)
		}
	})
	t.Run("artwork-only changes publish metadata", func(t *testing.T) {
		state.ArtworkURL = "https://example.test/updated.jpg"
		session.Update(state)
		var metadata map[string]dbus.Variant
		if err := wantProperty(t, "Metadata").Store(&metadata); err != nil {
			t.Fatal(err)
		}
		if got := metadata["mpris:artUrl"].Value(); got != state.ArtworkURL {
			t.Errorf("artwork = %v, want %s", got, state.ArtworkURL)
		}
		if got := metadata["xesam:title"].Value(); got != "First song" {
			t.Errorf("title = %v, want First song", got)
		}
		wantQuiet(t)
	})
	t.Run("volume bursts publish their final value", func(t *testing.T) {
		for _, volume := range []float64{0.2, 0.9, 0.1, 0.83} {
			state.Volume = volume
			session.Update(state)
		}
		if got := wantProperty(t, "Volume").Value(); got != 0.83 {
			t.Errorf("volume = %v, want 0.83", got)
		}
		value, err := object.GetProperty(playerInterface + ".Volume")
		if err != nil || value.Value() != 0.83 {
			t.Errorf("readable volume = %v, %v; want 0.83", value, err)
		}
		wantQuiet(t)
	})
	t.Run("returning volume to its published value is quiet", func(t *testing.T) {
		state.Volume = 0.2
		session.Update(state)
		state.Volume = 0.83
		session.Update(state)
		wantQuiet(t)
	})
	t.Run("remote volume is not echoed back", func(t *testing.T) {
		if err := object.Call("org.freedesktop.DBus.Properties.Set", 0, playerInterface, "Volume", dbus.MakeVariant(0.4)).Err; err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-volumes:
			if got != 0.4 {
				t.Errorf("volume command = %v, want 0.4", got)
			}
		case <-time.After(time.Second):
			t.Fatal("missing volume command")
		}
		if got := wantProperty(t, "Volume").Value(); got != 0.4 {
			t.Errorf("volume = %v, want 0.4", got)
		}
		state.Volume = 0.4
		session.Update(state)
		wantQuiet(t)
	})
	t.Run("remote shuffle and repeat are not reverted or echoed", func(t *testing.T) {
		for _, write := range []struct {
			property string
			value    any
		}{
			{"Shuffle", true},
			{"LoopStatus", "Playlist"},
		} {
			if err := object.Call("org.freedesktop.DBus.Properties.Set", 0, playerInterface, write.property, dbus.MakeVariant(write.value)).Err; err != nil {
				t.Fatal(err)
			}
			if got := wantProperty(t, write.property).Value(); got != write.value {
				t.Errorf("%s = %v, want %v", write.property, got, write.value)
			}
			// A poll can precede the asynchronous application callback.
			session.Update(state)
			wantQuiet(t)
			if write.property == "Shuffle" {
				state.Shuffle = true
			} else {
				state.LoopStatus = "Playlist"
			}
			session.Update(state)
			wantQuiet(t)
		}
	})
	t.Run("invalid remote writes do not change state", func(t *testing.T) {
		for _, write := range []struct {
			property string
			value    any
		}{
			{"Volume", "loud"},
			{"LoopStatus", "invalid"},
			{"PlaybackStatus", "Playing"},
		} {
			if err := object.Call("org.freedesktop.DBus.Properties.Set", 0, playerInterface, write.property, dbus.MakeVariant(write.value)).Err; err == nil {
				t.Errorf("invalid %s write succeeded", write.property)
			}
		}
		wantQuiet(t)
	})
	t.Run("empty tracks clear old metadata", func(t *testing.T) {
		state.VideoID, state.Title, state.Artist, state.ArtworkURL, state.Duration = "", "", "", "", 0
		session.Update(state)
		var metadata map[string]dbus.Variant
		if err := wantProperty(t, "Metadata").Store(&metadata); err != nil {
			t.Fatal(err)
		}
		if len(metadata) != 1 {
			t.Errorf("empty track retained metadata: %+v", metadata)
		}
		var properties map[string]dbus.Variant
		if err := object.Call("org.freedesktop.DBus.Properties.GetAll", 0, playerInterface).Store(&properties); err != nil {
			t.Fatal(err)
		}
		metadata = nil
		if err := properties["Metadata"].Store(&metadata); err != nil || len(metadata) != 1 {
			t.Errorf("GetAll retained metadata: %+v, error %v", metadata, err)
		}
		value, err := object.GetProperty(playerInterface + ".Metadata")
		if err != nil {
			t.Fatal(err)
		}
		metadata = nil
		if err := value.Store(&metadata); err != nil || len(metadata) != 1 {
			t.Errorf("Get retained metadata: %+v, error %v", metadata, err)
		}
		session.Update(state)
		wantQuiet(t)
	})
	t.Run("close cancels pending volume publications", func(t *testing.T) {
		state.Volume = 0.8
		session.Update(state)
		if err := session.Close(); err != nil {
			t.Fatal(err)
		}
		session.Update(state)
		wantQuiet(t)
	})
}
