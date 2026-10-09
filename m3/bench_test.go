package m3_test

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/elianiva/meiro/m3"
)

// BenchmarkHueSliderFrame measures a frame of the hue slider, whose track
// paints the whole spectrum: precomputing the strip should take the per-frame
// colour derivation out of it.
func BenchmarkHueSliderFrame(b *testing.B) {
	value := 200.0
	view := func(c *ui.Context) {
		ui.Column(c).Padding(20).Children(func() {
			m3.Slider(c, &value, 0, 360, m3.SliderSpec{Label: "Hue", Thickness: 16, Hue: true, Key: "hue"})
		})
	}
	tt := ui.NewTester(view, 600, 120)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tt.Frame()
	}
}

// BenchmarkSchemeMix measures gliding a scheme to another, which a theme
// change does once per frame: it should allocate nothing.
func BenchmarkSchemeMix(b *testing.B) {
	from := m3.NewScheme(m3.NewPalettes(m3.DefaultSeed, m3.TonalSpot), false)
	to := m3.NewScheme(m3.NewPalettes(m3.DefaultSeed, m3.Expressive), true)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = from.Mix(to, 0.5)
	}
}
