package main

import (
	"slices"
	"testing"

	"github.com/elianiva/meiro/youtube"
)

func TestSortPlaylistSections(t *testing.T) {
	items := []youtube.MusicItem{
		{VideoID: "long", Title: "Bravo", Subtitle: "Beta • Album", Duration: "10:00"},
		{VideoID: "short", Title: "alpha", Subtitle: "Alpha • Album", Duration: "2:00"},
		{VideoID: "duplicate", Title: "Alpha", Subtitle: "Alpha • Album", Duration: "2:00"},
	}
	sections := []youtube.MusicSection{{Title: "Songs", Items: items}}

	tests := []struct {
		name string
		sort playlistSort
		want []string
	}{
		{"title ascending", playlistSort{field: playlistSortTitle}, []string{"short", "duplicate", "long"}},
		{"title descending", playlistSort{field: playlistSortTitle, desc: true}, []string{"long", "short", "duplicate"}},
		{"artist ascending", playlistSort{field: playlistSortArtist}, []string{"short", "duplicate", "long"}},
		{"duration ascending", playlistSort{field: playlistSortDuration}, []string{"short", "duplicate", "long"}},
		{"duration descending", playlistSort{field: playlistSortDuration, desc: true}, []string{"long", "short", "duplicate"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sortPlaylistSections(sections, tt.sort)[0].Items
			ids := make([]string, len(got))
			for i := range got {
				ids[i] = got[i].VideoID
			}
			if !slices.Equal(ids, tt.want) {
				t.Fatalf("got %v, want %v", ids, tt.want)
			}
		})
	}
	if got := sortPlaylistSections(sections, playlistSort{}); !slices.Equal(got[0].Items, items) {
		t.Fatalf("original order changed: %v", got[0].Items)
	}
}

func TestPlaylistSortChooseResetsDirectionForNewField(t *testing.T) {
	s := playlistSort{field: playlistSortTitle, desc: true}
	s.choose(playlistSortArtist)
	if s.field != playlistSortArtist || s.desc {
		t.Fatalf("new field = %#v, want ascending artist", s)
	}
	s.choose(playlistSortOriginal)
	if s != (playlistSort{}) {
		t.Fatalf("original choice = %#v, want zero value", s)
	}
}
