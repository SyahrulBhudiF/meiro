//go:build darwin

package systemmedia

import (
	"errors"
	"sync"
	"time"

	"github.com/go-macos/objc"
)

const mediaPlayerFramework = "/System/Library/Frameworks/MediaPlayer.framework/MediaPlayer"

type macCommand struct {
	command objc.ID
	token   objc.ID
	block   objc.Block
}

type macSession struct {
	mu           sync.Mutex
	center       objc.ID
	remote       objc.ID
	commands     []macCommand
	state        State
	hasState     bool
	infoPosition time.Duration
	infoAt       time.Time
}

// New registers metadata and transport commands with macOS Now Playing.
func New(controls Controls) (Session, error) {
	if err := objc.Load(objc.Foundation, mediaPlayerFramework); err != nil {
		return nil, err
	}
	center := objc.ClassID("MPNowPlayingInfoCenter").Send(objc.Sel("defaultCenter"))
	remote := objc.ClassID("MPRemoteCommandCenter").Send(objc.Sel("sharedCommandCenter"))
	if center == 0 || remote == 0 {
		return nil, errors.New("macOS Now Playing services are unavailable")
	}
	s := &macSession{center: center, remote: remote}
	s.addCommand(remote, "playCommand", controls.Play, nil)
	s.addCommand(remote, "pauseCommand", controls.Pause, nil)
	s.addCommand(remote, "togglePlayPauseCommand", controls.Toggle, nil)
	s.addCommand(remote, "nextTrackCommand", controls.Next, nil)
	s.addCommand(remote, "previousTrackCommand", controls.Previous, nil)
	s.addCommand(remote, "changePlaybackPositionCommand", nil, controls.Seek)
	return s, nil
}

func (s *macSession) addCommand(remote objc.ID, selector string, action func(), seek func(time.Duration)) {
	command := remote.Send(objc.Sel(selector))
	if command == 0 || (action == nil && seek == nil) {
		return
	}
	block := objc.NewBlock(func(_ objc.Block, event objc.ID) int {
		if seek != nil {
			position := objc.Send[float64](event, objc.Sel("positionTime"))
			seek(time.Duration(position * float64(time.Second)))
		} else {
			action()
		}
		return 0 // MPRemoteCommandHandlerStatusSuccess
	})
	token := command.Send(objc.Sel("addTargetWithHandler:"), block)
	s.commands = append(s.commands, macCommand{command: command, token: token, block: block})
}

func (s *macSession) Update(state State) error {
	if state.VideoID == "" || state.Status == "" {
		state.Status = Stopped
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	objc.AutoreleasePool(func() {
		s.setEnabled("playCommand", state.CanPlay, s.state.CanPlay)
		s.setEnabled("pauseCommand", state.CanPause, s.state.CanPause)
		s.setEnabled("togglePlayPauseCommand", state.CanPlay || state.CanPause, s.state.CanPlay || s.state.CanPause)
		s.setEnabled("nextTrackCommand", state.CanNext, s.state.CanNext)
		s.setEnabled("previousTrackCommand", state.CanPrevious, s.state.CanPrevious)
		s.setEnabled("changePlaybackPositionCommand", state.CanSeek, s.state.CanSeek)

		metadataChanged := !s.hasState || !state.sameMetadata(s.state)
		statusChanged := !s.hasState || state.Status != s.state.Status
		positionChangedWhilePaused := state.Status != Playing && state.Position != s.state.Position
		seeked := s.hasState && state.SeekVersion != s.state.SeekVersion
		// Correct non-linear progress such as buffering, without publishing
		// again while the OS's extrapolated clock still agrees with the player.
		drifted := s.hasState && state.Status == Playing &&
			durationDistance(state.Position, s.infoPosition+time.Since(s.infoAt)) > time.Second
		if metadataChanged || statusChanged || positionChangedWhilePaused || seeked || drifted {
			s.publishInfo(state)
		}
		if statusChanged {
			playbackState := 3 // MPNowPlayingPlaybackStateStopped
			switch state.Status {
			case Playing:
				playbackState = 1
			case Paused:
				playbackState = 2
			}
			s.center.Send(objc.Sel("setPlaybackState:"), playbackState)
		}
	})
	s.state = state
	s.hasState = true
	return nil
}

func (s *macSession) setEnabled(selector string, enabled, previous bool) {
	if s.hasState && enabled == previous {
		return
	}
	command := s.remote.Send(objc.Sel(selector))
	if command != 0 {
		command.Send(objc.Sel("setEnabled:"), enabled)
	}
}

// publishInfo runs under mu. macOS advances elapsed time from playbackRate,
// so ordinary progress does not need another dictionary.
func (s *macSession) publishInfo(state State) {
	s.infoPosition, s.infoAt = state.Position, time.Now()
	if state.VideoID == "" {
		s.center.Send(objc.Sel("setNowPlayingInfo:"), 0)
		return
	}
	info := objc.ClassID("NSMutableDictionary").Send(objc.Sel("dictionary"))
	if state.Title != "" {
		info.Send(objc.Sel("setObject:forKey:"), objc.NSString(state.Title), objc.NSString("title"))
	}
	if state.Artist != "" {
		info.Send(objc.Sel("setObject:forKey:"), objc.NSString(state.Artist), objc.NSString("artist"))
	}
	if state.Duration > 0 {
		info.Send(objc.Sel("setObject:forKey:"), number(state.Duration.Seconds()), objc.NSString("playbackDuration"))
	}
	info.Send(objc.Sel("setObject:forKey:"), number(state.Position.Seconds()), objc.NSString("elapsedPlaybackTime"))
	rate := float64(0)
	if state.Status == Playing {
		rate = 1
	}
	info.Send(objc.Sel("setObject:forKey:"), number(rate), objc.NSString("playbackRate"))
	s.center.Send(objc.Sel("setNowPlayingInfo:"), info)
}

func number(value float64) objc.ID {
	return objc.ClassID("NSNumber").Send(objc.Sel("numberWithDouble:"), value)
}

func (s *macSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	objc.AutoreleasePool(func() {
		for _, target := range s.commands {
			if target.token != 0 {
				target.command.Send(objc.Sel("removeTarget:"), target.token)
			}
			target.block.Release()
		}
		s.commands = nil
		s.center.Send(objc.Sel("setNowPlayingInfo:"), 0)
		s.center.Send(objc.Sel("setPlaybackState:"), 3)
	})
	return nil
}
