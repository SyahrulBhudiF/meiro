package player

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCountingReaderCountsAndRemembersTheEnd(t *testing.T) {
	reader := newCountingReader(strings.NewReader("abcdef"))
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "abcdef" || reader.read.Load() != 6 {
		t.Errorf("read %q, counted %d", data, reader.read.Load())
	}
	if !reader.eof.Load() {
		t.Error("the end of the stream was not remembered")
	}
}

func TestBoundedBufferKeepsTheStart(t *testing.T) {
	buffer := &boundedBuffer{limit: 4}
	if _, err := buffer.Write([]byte("abcdefgh")); err != nil {
		t.Fatal(err)
	}
	if got := buffer.String(); got != "abcd" {
		t.Errorf("boundedBuffer = %q, want %q", got, "abcd")
	}
}

func TestIdlePlayer(t *testing.T) {
	p := New()
	if p.Active() || p.Playing() || p.Paused() || p.Ended() {
		t.Error("a new player is not idle")
	}
	if p.Position() != 0 || p.Failure() != "" {
		t.Errorf("position %v, failure %q", p.Position(), p.Failure())
	}
	p.Pause()
	p.Resume()
	p.Toggle()
	p.Seek(time.Minute)
	p.Stop()
}

func TestPlayRejectsAnEmptyURL(t *testing.T) {
	if err := New().Play("  "); err == nil {
		t.Fatal("playing an empty URL should fail")
	}
}

func TestAvailableFollowsFFmpeg(t *testing.T) {
	p := New()
	_, err := p.resolveFFmpeg()
	if got := p.Available(); got != (err == nil) {
		t.Errorf("Available = %v, but resolving ffmpeg gave %v", got, err)
	}
	if err != nil && !errors.Is(err, ErrNoFFmpeg) {
		t.Errorf("resolveFFmpeg error = %v, want ErrNoFFmpeg", err)
	}
}

func TestVolumeIsClamped(t *testing.T) {
	p := New()
	p.SetVolume(-1)
	if p.Volume() != 0 {
		t.Errorf("volume = %v after a negative value", p.Volume())
	}
	p.SetVolume(0.5)
	if p.Volume() != 0.5 {
		t.Errorf("volume = %v", p.Volume())
	}
}

func TestBoundedBufferNeverGrowsPastItsLimit(t *testing.T) {
	buffer := &boundedBuffer{limit: 8}
	for range 100 {
		_, _ = buffer.Write(bytes.Repeat([]byte("x"), 32))
	}
	if len(buffer.String()) != 8 {
		t.Errorf("buffer holds %d bytes, want 8", len(buffer.String()))
	}
}

// endOfStream decides whether a track has been played to its end. Its truth
// table is the whole contract the UI advances tracks on: a pause near the
// end holds the track, and only an unpaused stream with its data read out
// and the device idle is over.
func TestEndOfStream(t *testing.T) {
	const (
		idleDevice    = false // the device player reports not playing
		playingDevice = true
		held          = true  // the stream is paused
		following     = false // the stream is not paused
		noneRead      = int64(0)
		someRead      = int64(1024)
	)
	for _, tt := range []struct {
		name      string
		read      int64
		eof       bool
		isPlaying bool
		paused    bool
		want      bool
	}{
		{"nothing read is not the end", noneRead, true, idleDevice, following, false},
		{"no end of data is not the end", someRead, false, idleDevice, following, false},
		{"a playing stream is not over", someRead, true, playingDevice, following, false},
		{"a paused stream at its end is held, not ended", someRead, true, idleDevice, held, false},
		{"a paused stream before its end is held", someRead, false, playingDevice, held, false},
		{"a stream read out with the device idle has ended", someRead, true, idleDevice, following, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := endOfStream(tt.read, tt.eof, tt.isPlaying, tt.paused); got != tt.want {
				t.Errorf("endOfStream(read=%d, eof=%v, playing=%v, paused=%v) = %v, want %v", tt.read, tt.eof, tt.isPlaying, tt.paused, got, tt.want)
			}
		})
	}
}

func TestFailureReportsAnErrorAfterSound(t *testing.T) {
	newFinishedPlayer := func() (*Player, *session) {
		done := make(chan struct{})
		close(done)
		source := newCountingReader(strings.NewReader(""))
		source.read.Store(1024)
		s := &session{source: source, stderr: &boundedBuffer{limit: 16}, done: done}
		return &Player{session: s}, s
	}

	t.Run("negative: clean exit after sound", func(t *testing.T) {
		p, _ := newFinishedPlayer()
		if got := p.Failure(); got != "" {
			t.Errorf("clean exit after sound is the track ending, got failure %q", got)
		}
	})
	t.Run("positive: stderr error after sound", func(t *testing.T) {
		p, s := newFinishedPlayer()
		if _, err := s.stderr.Write([]byte("read error")); err != nil {
			t.Fatal(err)
		}
		if got := p.Failure(); !strings.Contains(got, "read error") {
			t.Errorf("error after sound was not reported, got failure %q", got)
		}
	})
	t.Run("positive: exit status error", func(t *testing.T) {
		p, s := newFinishedPlayer()
		s.waitErr = errors.New("exit status 1")
		if got := p.Failure(); got == "" {
			t.Error("ffmpeg dying with an error is a failure")
		}
	})
}

// ffmpeg's reconnect and user-agent options belong to the HTTP protocol. A
// cached track is a local file, and ffmpeg refuses to open an input when it
// is given an option its protocol does not define, so a cached track used to
// play nothing at all.
func TestStreamArgsOnlySendHTTPOptionsToHTTPURLs(t *testing.T) {
	local := streamArgs("/Users/me/Meiro Audio Cache/abc.webm", 0)
	for _, arg := range local {
		switch arg {
		case "-reconnect", "-reconnect_streamed", "-reconnect_delay_max", "-rw_timeout", "-user_agent":
			t.Errorf("a local file was given the HTTP option %s: %v", arg, local)
		}
	}
	remote := streamArgs("https://media.example/videoplayback?id=1", 0)
	joined := strings.Join(remote, " ")
	for _, want := range []string{"-xerror", "-reconnect 1", "-reconnect_streamed 1", "-reconnect_delay_max 5", "-rw_timeout 15000000", "-user_agent"} {
		if !strings.Contains(joined, want) {
			t.Errorf("an HTTP stream did not get %q: %v", want, remote)
		}
	}
	if !strings.Contains(strings.Join(local, " "), "-xerror") {
		t.Errorf("a local stream did not get -xerror: %v", local)
	}
	// Seeking a local file still works: -ss is an input option, before -i.
	seeking := strings.Join(streamArgs("/tmp/track.webm", 90*time.Second), " ")
	if !strings.Contains(seeking, "-ss 90.000") || !strings.Contains(seeking, "-i /tmp/track.webm") {
		t.Errorf("a seek on a local file lost its offset or input: %s", seeking)
	}
}

// isHTTP decides which inputs the HTTP-only options may be given, so a path
// with a drive letter, a bare file name, or another protocol must not match.
func TestIsHTTPRecognisesOnlyTheStreamingSchemes(t *testing.T) {
	for _, test := range []struct {
		rawURL string
		want   bool
	}{
		{"http://media.example/a", true},
		{"https://media.example/a?dur=3", true},
		{"HTTPS://media.example/a", true},
		{"rtsp://camera/a", false},
		{"/Users/me/Meiro Audio Cache/abc.webm", false},
		{"track.webm", false},
		{"C:\\Meiro Audio Cache\\abc.webm", false},
		{"", false},
	} {
		if got := isHTTP(test.rawURL); got != test.want {
			t.Errorf("isHTTP(%q) = %v, want %v", test.rawURL, got, test.want)
		}
	}
}

type fakeOutput struct {
	mu       sync.Mutex
	err      error
	reader   io.Reader
	playing  bool
	buffered int
	stopped  bool
}

func (f *fakeOutput) Play() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playing = true
}

func (f *fakeOutput) Pause() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playing = false
}

func (f *fakeOutput) SetVolume(float64) {}

func (f *fakeOutput) BufferedSize() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buffered
}

func (f *fakeOutput) IsPlaying() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.playing
}

func (f *fakeOutput) PauseAndStopReading() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playing = false
	f.stopped = true
}

func (f *fakeOutput) Err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

func (f *fakeOutput) setBuffered(size int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.buffered = size
}

func (f *fakeOutput) setError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeOutput) drain(t *testing.T) {
	t.Helper()
	if _, err := io.Copy(io.Discard, f.reader); err != nil {
		t.Fatal(err)
	}
	f.Pause()
}

type fakeDevice struct {
	mu      sync.Mutex
	err     error
	outputs []*fakeOutput
}

func (d *fakeDevice) newPlayer(reader io.Reader) output {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := &fakeOutput{reader: reader}
	d.outputs = append(d.outputs, out)
	return out
}

func (d *fakeDevice) Err() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.err
}

func (d *fakeDevice) lastOutput(t *testing.T) *fakeOutput {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.outputs) == 0 {
		t.Fatal("ffmpeg started without creating an output")
	}
	return d.outputs[len(d.outputs)-1]
}

func fakeAudio(fake *fakeDevice) *sharedAudio {
	return &sharedAudio{open: func() (device, error) { return fake, nil }}
}

func testFFmpeg(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestPlayer(t *testing.T, script string) (*Player, *fakeDevice) {
	t.Helper()
	device := &fakeDevice{}
	p := New(WithFFmpeg(script), withAudio(fakeAudio(device)))
	t.Cleanup(p.Stop)
	return p, device
}

func sessionOf(t *testing.T, p *Player) *session {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session == nil {
		t.Fatal("player has no session")
	}
	return p.session
}

func waitClosed(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the ffmpeg process to exit")
	}
}

func waitForFailure(t *testing.T, p *Player) string {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if failure := p.Failure(); failure != "" {
			return failure
		}
		select {
		case <-deadline.C:
			t.Fatal("timed out waiting for player failure")
		case <-tick.C:
		}
	}
}

func waitForFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			return data
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", path)
		case <-tick.C:
		}
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\\"'\\\"'") + "'"
}

func TestStateTransitionsWithFakeOutput(t *testing.T) {
	p, device := newTestPlayer(t, testFFmpeg(t, "printf samples"))
	if err := p.Play("track"); err != nil {
		t.Fatal(err)
	}
	out := device.lastOutput(t)
	if !p.Active() || !p.Playing() || p.Paused() {
		t.Fatal("Play did not start an active, playing session")
	}
	p.Pause()
	if p.Playing() || !p.Paused() {
		t.Fatal("Pause did not hold the session")
	}
	p.Resume()
	p.Toggle()
	if !p.Paused() {
		t.Fatal("Toggle did not pause the session")
	}
	p.Toggle()
	if !p.Playing() || p.Paused() {
		t.Fatal("Toggle did not resume the session")
	}
	out.drain(t)
	waitClosed(t, sessionOf(t, p).done)
	if !p.Ended() {
		t.Fatal("a drained, clean stream was not ended")
	}
}

func TestSeekPreservesPauseAndPosition(t *testing.T) {
	args := filepath.Join(t.TempDir(), "args")
	body := "printf '%s\\n' \"$@\" > " + shellQuote(args) + "\nprintf samples\nexec sleep 60"
	p, device := newTestPlayer(t, testFFmpeg(t, body))
	if err := p.Play("track"); err != nil {
		t.Fatal(err)
	}
	first := sessionOf(t, p)
	first.source.read.Store(3 * bytesPerSecond)
	device.lastOutput(t).setBuffered(bytesPerSecond)
	if got, want := p.Position(), 2*time.Second; got != want {
		t.Errorf("Position = %v, want %v", got, want)
	}
	p.Pause()
	_ = waitForFile(t, args)
	if err := os.Remove(args); err != nil {
		t.Fatal(err)
	}
	p.Seek(2500 * time.Millisecond)
	second := sessionOf(t, p)
	if second == first || !p.Paused() {
		t.Fatal("Seek did not replace and preserve the paused session")
	}
	waitClosed(t, first.done)
	second.source.read.Store(3 * bytesPerSecond)
	device.lastOutput(t).setBuffered(bytesPerSecond / 2)
	if got, want := p.Position(), 5*time.Second; got != want {
		t.Errorf("Position after seek = %v, want %v", got, want)
	}
	got := strings.Fields(string(waitForFile(t, args)))
	ss, input := -1, -1
	for i, arg := range got {
		if arg == "-ss" {
			ss = i
		}
		if arg == "-i" {
			input = i
		}
	}
	if ss < 0 || input < 0 || ss > input || got[ss+1] != "2.500" {
		t.Errorf("ffmpeg arguments = %q, want -ss 2.500 before -i", got)
	}
}

func TestFailureAndReplacementReapFakeFFmpeg(t *testing.T) {
	t.Run("failure", func(t *testing.T) {
		p, device := newTestPlayer(t, testFFmpeg(t, "echo unavailable >&2\nexit 17"))
		if err := p.Play("track"); err != nil {
			t.Fatal(err)
		}
		device.lastOutput(t).drain(t)
		if got := waitForFailure(t, p); !strings.Contains(got, "unavailable") {
			t.Errorf("Failure = %q, want ffmpeg stderr", got)
		}
		if sessionOf(t, p).waitErr == nil {
			t.Error("an ffmpeg exit status was not recorded")
		}
	})
	t.Run("replacement", func(t *testing.T) {
		p, _ := newTestPlayer(t, testFFmpeg(t, "printf samples\nexec sleep 60"))
		if err := p.Play("first"); err != nil {
			t.Fatal(err)
		}
		first := sessionOf(t, p)
		if err := p.Play("second"); err != nil {
			t.Fatal(err)
		}
		waitClosed(t, first.done)
		if first.cmd.ProcessState == nil {
			t.Fatal("replaced ffmpeg process was not reaped")
		}
	})
}

func TestMissingFFmpegIsReported(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	p, _ := newTestPlayer(t, "")
	if err := p.Play("track"); !errors.Is(err, ErrNoFFmpeg) {
		t.Fatalf("Play error = %v, want ErrNoFFmpeg", err)
	}
}

func TestParallelStateAccess(t *testing.T) {
	p, _ := newTestPlayer(t, testFFmpeg(t, "printf samples\nexec sleep 60"))
	if err := p.Play("track"); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for _, action := range []func(){
		func() { p.Pause(); p.Resume(); p.Toggle() },
		func() { _ = p.Position(); _ = p.Playing(); _ = p.Ended(); _ = p.Failure() },
		p.Stop,
	} {
		group.Go(func() {
			for range 100 {
				action()
			}
		})
	}
	group.Wait()
}

func TestPlayersShareOneContext(t *testing.T) {
	opened := 0
	shared := &sharedAudio{open: func() (device, error) { opened++; return &fakeDevice{}, nil }}
	a, b := &Player{audio: shared}, &Player{audio: shared}
	if _, err := a.audio.context(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.audio.context(); err != nil {
		t.Fatal(err)
	}
	if opened != 1 {
		t.Errorf("device opened %d times, want 1", opened)
	}
}

func TestOpenErrorIsReportedAndKept(t *testing.T) {
	opened := 0
	shared := &sharedAudio{open: func() (device, error) { opened++; return nil, errors.New("no device") }}
	p := &Player{audio: shared, ffmpegPath: "ffmpeg"}
	p.lookOnce.Do(func() {})
	for range 2 {
		if err := p.Play("http://example.invalid/x"); err == nil {
			t.Fatal("want an error when the device cannot open")
		}
	}
	if opened != 1 {
		t.Errorf("open attempted %d times, want 1", opened)
	}
}

func TestOutputErrorBecomesFailure(t *testing.T) {
	out := &fakeOutput{playing: true}
	source := newCountingReader(strings.NewReader(""))
	s := &session{source: source, stderr: &boundedBuffer{limit: 16}, done: make(chan struct{}), out: out}
	p := &Player{session: s, audio: &sharedAudio{open: func() (device, error) { return &fakeDevice{}, nil }}}
	if got := p.Failure(); got != "" {
		t.Fatalf("unexpected failure %q", got)
	}
	out.setError(errors.New("device lost"))
	if got := p.Failure(); !strings.Contains(got, "device lost") {
		t.Errorf("failure = %q, want the output error", got)
	}
	if p.Playing() {
		t.Error("a player whose output failed must not report playing")
	}

	out.setError(nil)
	p.audio = &sharedAudio{open: func() (device, error) { return &fakeDevice{err: errors.New("ctx dead")}, nil }}
	if got := p.Failure(); !strings.Contains(got, "ctx dead") {
		t.Errorf("failure = %q, want the context error", got)
	}
}

// A stream that fails mid-track is told apart from a track that ended and from
// the sound going away: the app moves the queue on for the first, and leaves it
// alone for the others.
func TestStreamFailedTellsAReadFailureFromACleanEnd(t *testing.T) {
	failing := filepath.Join(t.TempDir(), "ffmpeg-fails")
	body := "#!/bin/sh\nhead -c 65536 /dev/zero\necho 'IO error: Connection reset by peer' >&2\nexit 1\n"
	if err := os.WriteFile(failing, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	p := New(WithFFmpeg(failing), WithSilentAudio())
	t.Cleanup(p.Stop)
	if err := p.Play("http://example.invalid/reset"); err != nil {
		t.Fatal(err)
	}
	waitForStop(t, p)
	if !p.StreamFailed() {
		t.Errorf("a failed read was not reported as a stream failure: %q", p.Failure())
	}

	clean := filepath.Join(t.TempDir(), "ffmpeg-clean")
	body = "#!/bin/sh\nhead -c 65536 /dev/zero\nexit 0\n"
	if err := os.WriteFile(clean, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	p = New(WithFFmpeg(clean), WithSilentAudio())
	t.Cleanup(p.Stop)
	if err := p.Play("http://example.invalid/clean"); err != nil {
		t.Fatal(err)
	}
	waitForStop(t, p)
	if p.Failure() != "" {
		t.Errorf("a clean end was reported as a failure: %q", p.Failure())
	}
	if p.StreamFailed() {
		t.Error("a clean end was reported as a stream failure")
	}
}

// waitForStop waits until the decode has stopped, and says so when it does
// not.
func waitForStop(t *testing.T, p *Player) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if p.Ended() || p.Failure() != "" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the decode never stopped: playing=%v failure=%q", p.Playing(), p.Failure())
}
