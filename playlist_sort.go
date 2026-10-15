package main

import (
	"cmp"
	"sort"
	"strconv"
	"strings"

	"github.com/elianiva/meiro/youtube"
)

// playlistSortField is the value the playlist page orders its tracks by.
type playlistSortField uint8

const (
	playlistSortOriginal playlistSortField = iota
	playlistSortTitle
	playlistSortArtist
	playlistSortDuration
	playlistSortDateAdded
)

// playlistSort is one playlist page's ordering choice. Original is the zero
// value, so an untouched page keeps YouTube Music's order.
type playlistSort struct {
	field playlistSortField
	desc  bool
}

func (s playlistSort) active() bool { return s.field != playlistSortOriginal }

func (s playlistSort) label() string {
	switch s.field {
	case playlistSortOriginal:
		return "Original order"
	case playlistSortTitle:
		return "Title"
	case playlistSortArtist:
		return "Artist"
	case playlistSortDuration:
		return "Duration"
	case playlistSortDateAdded:
		return "Date added"
	}
	return "Original order"
}

func (s playlistSort) directionLabel() string {
	if !s.active() {
		return "Order"
	}
	if s.desc {
		switch s.field {
		case playlistSortTitle, playlistSortArtist:
			return "Z–A"
		case playlistSortOriginal:
			return "Order"
		case playlistSortDuration:
			return "Longest first"
		case playlistSortDateAdded:
			return "Newest first"
		}
	}
	switch s.field {
	case playlistSortOriginal:
		return "Order"
	case playlistSortTitle, playlistSortArtist:
		return "A–Z"
	case playlistSortDuration:
		return "Shortest first"
	case playlistSortDateAdded:
		return "Oldest first"
	}
	return "Order"
}

func (s *playlistSort) choose(field playlistSortField) {
	if field == playlistSortOriginal {
		*s = playlistSort{}
		return
	}
	if s.field != field {
		s.field, s.desc = field, false
		return
	}
	s.desc = !s.desc
}

// sortPlaylistSections returns the playlist tracks in one stable order. The
// original slice is never modified, and equal or missing values retain their
// YouTube order. This also keeps duplicate playlist entries distinct.
func sortPlaylistSections(sections []youtube.MusicSection, s playlistSort) []youtube.MusicSection {
	if !s.active() {
		return sections
	}
	tracks := make([]youtube.MusicItem, 0)
	for _, section := range sections {
		for _, item := range section.Items {
			if item.IsTrack() {
				tracks = append(tracks, item)
			}
		}
	}
	sort.SliceStable(tracks, func(i, j int) bool {
		left, right := tracks[i], tracks[j]
		order := comparePlaylistItems(left, right, s.field)
		if s.desc {
			return order > 0
		}
		return order < 0
	})
	return []youtube.MusicSection{{Items: tracks}}
}

func comparePlaylistItems(left, right youtube.MusicItem, field playlistSortField) int {
	var order int
	switch field {
	case playlistSortOriginal:
		return 0
	case playlistSortTitle:
		order = strings.Compare(strings.ToLower(left.Title), strings.ToLower(right.Title))
	case playlistSortArtist:
		order = strings.Compare(strings.ToLower(playlistArtist(left)), strings.ToLower(playlistArtist(right)))
	case playlistSortDuration:
		order = cmp.Compare(durationSeconds(left.Duration), durationSeconds(right.Duration))
	case playlistSortDateAdded:
		// InnerTube playlist browse responses do not expose a per-entry date.
		// Keeping this comparison equal is deliberate: position is not a date.
		return 0
	}
	if order != 0 {
		return order
	}
	return strings.Compare(strings.ToLower(left.Title), strings.ToLower(right.Title))
}

// playlistArtist follows the text convention already used by the parser:
// artist, album and duration are separated by " • ".
func playlistArtist(item youtube.MusicItem) string {
	artist, _, _ := strings.Cut(item.Subtitle, " • ")
	return strings.TrimSpace(artist)
}

func durationSeconds(value string) int {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) == 0 || len(parts) > 3 {
		return 0
	}
	seconds := 0
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return 0
		}
		seconds = seconds*60 + n
	}
	return seconds
}
