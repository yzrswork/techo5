package yzrs

import (
	"golang.org/x/image/font"
	"image"
	"strings"
	"testing"
	"time"
)

func TestPolishCountAndRemainingRing(t *testing.T) {
	for _, tc := range []struct {
		n    int64
		want string
	}{{0, "0"}, {2195, "2,195"}, {8423189, "8,423,189"}, {9007199254740991, "9,007,199,254,740,991"}} {
		if got := Count(&tc.n); got != tc.want {
			t.Fatalf("Count: %s, want %s", got, tc.want)
		}
	}
	if Count(nil) != "—" {
		t.Fatal("unknown count fabricated")
	}
	for _, v := range []float64{0, 25, 100} {
		img := image.NewRGBA(image.Rect(0, 0, 100, 100))
		drawRing(img, 50, 50, 40, 5, &v, cyan)
		// Clockwise remaining arc: right edge filled at 100%, left empty at 25%.
		if v == 0 && img.RGBAAt(90, 50) == cyan {
			t.Fatal("zero remaining painted full")
		}
		if v == 100 && img.RGBAAt(10, 50) != cyan {
			t.Fatal("full remaining arc missing")
		}
		if v == 25 && img.RGBAAt(10, 50) == cyan {
			t.Fatal("consumed portion painted as remaining")
		}
	}
	for i := range Modes {
		b := NavBounds(i)
		for _, p := range []image.Point{b.Min, image.Pt(b.Max.X-1, b.Max.Y-1)} {
			m, ok := ModeAt(p.X, p.Y)
			if !ok || m != i {
				t.Fatal("tile edge not tappable")
			}
		}
		if _, ok := ModeAt(b.Max.X, b.Min.Y); ok {
			t.Fatal("gap activated navigation")
		}
	}
}

func TestPolishJapaneseWrappingAndWeatherFallback(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	text := strings.Repeat("日本語の長い活動内容", 5)
	lines := wrapTitle(text, r.normal, 582)
	if len(lines) != 2 || strings.Join(lines, "") != text || font.MeasureString(r.normal, lines[0]).Ceil() > 582 {
		t.Fatal("wrapped source content changed")
	}
	base := Frame{Now: fixtureTime, Weather: Weather{Condition: "sunny", Temp: "20°", Updated: fixtureTime.Add(-4 * time.Hour)}}
	stale := image.NewRGBA(image.Rect(0, 0, 960, 480))
	empty := image.NewRGBA(stale.Bounds())
	r.Draw(stale, base)
	base.Weather = Weather{}
	r.Draw(empty, base)
	if string(stale.Pix) != string(empty.Pix) {
		t.Fatal("expired weather did not use neutral fallback")
	}
}
