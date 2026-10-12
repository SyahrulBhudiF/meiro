package player

import (
	"io"
	"sync"
)

// WithSilentAudio makes the player discard its samples instead of opening the
// system's audio device. Decoding, buffering and stream errors behave as they
// do with real sound, so it suits tests and headless machines where oto cannot
// open a device.
func WithSilentAudio() Option {
	dev := &silentDevice{}
	return withAudio(&sharedAudio{open: func() (device, error) { return dev, nil }})
}

type silentDevice struct{}

func (*silentDevice) newPlayer(r io.Reader) output { return &silentOutput{reader: r} }
func (*silentDevice) Err() error                   { return nil }

// silentOutput reads its source at full speed while playing and stops at the
// end of the stream, as an idle device player does.
type silentOutput struct {
	reader io.Reader

	mu      sync.Mutex
	playing bool
	stopped bool
	reading bool
}

func (o *silentOutput) Play() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stopped {
		return
	}
	o.playing = true
	if o.reading {
		return
	}
	o.reading = true
	go o.drain()
}

func (o *silentOutput) drain() {
	buf := make([]byte, 4096)
	for {
		o.mu.Lock()
		if o.stopped || !o.playing {
			o.reading = false
			o.mu.Unlock()
			return
		}
		o.mu.Unlock()
		if _, err := o.reader.Read(buf); err != nil {
			o.mu.Lock()
			o.playing = false
			o.reading = false
			o.mu.Unlock()
			return
		}
	}
}

func (o *silentOutput) Pause() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.playing = false
}

func (*silentOutput) SetVolume(float64) {}
func (*silentOutput) BufferedSize() int { return 0 }
func (*silentOutput) Err() error        { return nil }

func (o *silentOutput) IsPlaying() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.playing
}

func (o *silentOutput) PauseAndStopReading() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.playing = false
	o.stopped = true
}
