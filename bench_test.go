package main

import (
	"fmt"
	"image"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/elianiva/meiro/youtube"
)

// benchAlbum is an album card such as a shelf shows.
func benchAlbum(i int) youtube.MusicItem {
	id := strconv.Itoa(i)
	return youtube.MusicItem{
		ID: "MPREb_" + id, BrowseID: "MPREb_" + id, Kind: "music_item",
		Title: "Album " + id, Subtitle: "Album • Aurora Vale", Thumbnail: "https://art.test/albums/" + id,
	}
}

// benchTracks returns n songs, as a page of them lists.
func benchTracks(n int) []youtube.MusicItem {
	items := make([]youtube.MusicItem, n)
	for i := range items {
		id := strconv.Itoa(i)
		items[i] = youtube.MusicItem{
			ID: id, VideoID: "v" + id, Kind: "track",
			Title: "Song " + id, Subtitle: "Aurora Vale", Duration: "3:00",
			Thumbnail: "https://art.test/songs/" + id,
		}
	}
	return items
}

// benchShelfApp shows n album cards in one shelf, with artwork of its own so
// the benchmark needs no network.
func benchShelfApp(n int) *app {
	a := newTestApp()
	a.location = "/home"
	items := make([]youtube.MusicItem, n)
	for i := range items {
		items[i] = benchAlbum(i)
	}
	a.feed = pageState{sections: []youtube.MusicSection{{Title: "Albums", Items: items}}}
	art := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 8, 8)))
	a.thumbs.synth = func(string, int) *ui.Bitmap { return art }
	return a
}

// BenchmarkPageFrame measures one frame of a page holding a shelf of cards.
// Building only the cards in view should keep it flat as the shelf grows.
func BenchmarkPageFrame(b *testing.B) {
	for _, n := range []int{50, 500, 1000} {
		b.Run(fmt.Sprintf("cards=%d", n), func(b *testing.B) {
			a := benchShelfApp(n)
			tt := ui.NewTester(a.view, 1000, 700)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tt.Frame()
			}
		})
	}
}

// BenchmarkQuickPicksFrame measures one frame of a page holding a shelf of
// songs in columns, as Quick picks is. Building only the columns in view
// should keep it flat as the shelf grows.
func BenchmarkQuickPicksFrame(b *testing.B) {
	for _, n := range []int{50, 500, 4096} {
		b.Run(fmt.Sprintf("songs=%d", n), func(b *testing.B) {
			a := newTestApp()
			a.location = "/home"
			a.feed = pageState{sections: []youtube.MusicSection{{Title: "Quick picks", Items: benchTracks(n)}}}
			art := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 8, 8)))
			a.thumbs.synth = func(string, int) *ui.Bitmap { return art }
			tt := ui.NewTester(a.view, 1000, 700)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tt.Frame()
			}
		})
	}
}

// BenchmarkSettledFrame measures a frame of an idle page that has not
// changed, where the flattened rows are expected to be reused.
func BenchmarkSettledFrame(b *testing.B) {
	for _, n := range []int{500, 5000} {
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			a := newTestApp()
			a.router.Push("/search")
			a.location = "/search"
			a.search.submitted = "x"
			a.search.items = benchTracks(n)
			tt := ui.NewTester(a.view, 1000, 700)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tt.Frame()
			}
		})
	}
}

// BenchmarkSetRows measures flattening a loaded page into the rows its list
// shows, the work a frame should not repeat when nothing changed.
func BenchmarkSetRows(b *testing.B) {
	for _, n := range []int{500, 5000} {
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			a := newTestApp()
			a.router.Push("/search")
			a.location = "/search"
			a.search.submitted = "x"
			a.search.items = benchTracks(n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				a.setRows()
			}
		})
	}
}

// BenchmarkSettingsFrame measures a frame of the settings page, where the
// theme and every palette preview are resolved and the hue slider paints.
func BenchmarkSettingsFrame(b *testing.B) {
	a := newTestApp()
	a.router.Push("/settings")
	tt := ui.NewTester(a.view, 1000, 900)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tt.Frame()
	}
}
