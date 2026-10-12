package systemmedia

import (
	"testing"
	"time"
)

func TestStateSameMetadata(t *testing.T) {
	state := State{
		VideoID: "video", Title: "Title", Artist: "Artist", ArtworkURL: "https://example.test/art.jpg", Duration: 3 * time.Minute,
	}
	if !state.sameMetadata(state) {
		t.Fatal("an identical state has different metadata")
	}
	for _, change := range []struct {
		name  string
		apply func(*State)
	}{
		{"video ID", func(s *State) { s.VideoID = "other" }},
		{"title", func(s *State) { s.Title = "Other" }},
		{"artist", func(s *State) { s.Artist = "Other" }},
		{"artwork URL", func(s *State) { s.ArtworkURL = "https://example.test/other.jpg" }},
		{"duration", func(s *State) { s.Duration = 4 * time.Minute }},
	} {
		t.Run(change.name, func(t *testing.T) {
			changed := state
			change.apply(&changed)
			if state.sameMetadata(changed) {
				t.Errorf("metadata change was treated as equal: %+v", changed)
			}
		})
	}
}
