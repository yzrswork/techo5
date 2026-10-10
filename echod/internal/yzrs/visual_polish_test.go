package yzrs

import (
	"bytes"
	"image"
	"testing"

	"golang.org/x/image/font"
)

func TestVisualFontCoverageAndFit(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for ch := rune(32); ch <= 126; ch++ {
		if _, _, ok := r.latin.GlyphBounds(ch); !ok {
			t.Errorf("missing ASCII glyph %q", ch)
		}
	}
	for _, text := range []string{"日本語の活動", "10月10日", "F8を押して話す"} {
		if r.faceFor(text, r.normal) != r.normal {
			t.Errorf("Japanese fallback changed: %q", text)
		}
	}
	for _, text := range []string{"99,999,999", "UNAVAILABLE", "OFFLINE / LKG", "CODEX TEMP"} {
		base := r.small
		if text == "99,999,999" || text == "UNAVAILABLE" {
			base = r.normal
		}
		face := r.faceFor(text, base)
		if face == base {
			t.Fatalf("ASCII face not selected: %q", text)
		}
		if width := font.MeasureString(face, text).Ceil(); width > hudWidth {
			t.Errorf("HUD clips %q: %d", text, width)
		}
	}
	for _, text := range []string{"00:00", "12:34", "23:59"} {
		bounds, advance := font.BoundString(r.large, text)
		if advance.Ceil() > 676 || bounds.Min.X.Floor() < 0 || bounds.Min.Y.Floor() < -112 || bounds.Max.Y.Ceil() > 0 {
			t.Errorf("clock glyph bounds do not fit: %q %v", text, bounds)
		}
	}
}

func TestVoicePresentationPreservesPTTAndIgnoresDeck(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	frame := Frame{Now: fixtureTime, Mode: 3, Data: View{Status: "NO DATA"}, Voice: PTTView{Connected: true, Phase: "IDLE"}, Deck: "UNAVAILABLE"}
	draw := func(f Frame) []byte {
		img := image.NewRGBA(image.Rect(0, 0, 960, 480))
		r.Draw(img, f)
		return img.Pix
	}
	idle := draw(frame)
	frame.Deck = "PAIRED PC READY"
	if !bytes.Equal(idle, draw(frame)) {
		t.Fatal("Deck still affects VOICE presentation")
	}
	for _, phase := range []string{"LISTENING", "TRANSCRIBING"} {
		frame.Voice.Phase = phase
		if bytes.Equal(idle, draw(frame)) {
			t.Errorf("PTT phase %s is not visible", phase)
		}
	}
	frame.Voice = PTTView{Phase: "IDLE"}
	if bytes.Equal(idle, draw(frame)) {
		t.Fatal("PC connection status is not visible")
	}
}

func BenchmarkVisualDraw(b *testing.B) {
	r, err := NewRenderer()
	if err != nil {
		b.Fatal(err)
	}
	defer r.Close()
	img := image.NewRGBA(image.Rect(0, 0, 960, 480))
	frame := Frame{Now: fixtureTime, Data: View{Status: "OFFLINE / LKG"}, Mode: 0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frame.Mode = i % 4
		r.Draw(img, frame)
	}
}
