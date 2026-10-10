package yzrs

import (
	"bytes"
	"image"
	"testing"
	"time"

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
	for _, text := range []string{"00:00", "08:35", "11:11", "12:34", "17:38", "23:59"} {
		bounds, advance := font.BoundString(r.large, text)
		left := 250 + max(0, (676-advance.Ceil())/2)
		// Curved digits may extend below the baseline; validate the actual
		// clock area above the date rather than requiring zero overshoot.
		if advance.Ceil() > 676 || left+bounds.Min.X.Floor() < 250 || left+bounds.Max.X.Ceil() > 926 || 218+bounds.Min.Y.Floor() < 80 || 218+bounds.Max.Y.Ceil() >= 244 {
			t.Errorf("clock glyph bounds do not fit: %q %v", text, bounds)
		}
	}
}

func TestVaultFreshnessLabelsAndLayout(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, tc := range []struct{ internal, shown string }{
		{"LIVE", "VAULT OK"},
		{"STALE", "VAULT STALE"},
		{"OFFLINE / LKG", "OFFLINE / LKG"},
		{"NO DATA", "NO DATA"},
	} {
		if got := vaultHUDStatus(tc.internal); got != tc.shown {
			t.Fatalf("Vault HUD status %q -> %q, want %q", tc.internal, got, tc.shown)
		}
		if width := font.MeasureString(r.faceFor(tc.shown, r.small), tc.shown).Ceil(); width > hudWidth {
			t.Fatalf("status does not fit HUD: %q %dpx", tc.shown, width)
		}
	}
	for _, tc := range []struct {
		generated time.Time
		stamp     string
		heading   string
	}{
		{time.Date(2026, 10, 10, 23, 55, 0, 0, JST), "SNAP 10/10 23:55", "TODAY / 10月10日の活動"},
		{time.Date(2026, 1, 1, 8, 0, 0, 0, JST), "SNAP 01/01 08:00", "TODAY / 1月1日の活動"},
	} {
		s := &Snapshot{Generated: tc.generated}
		if got := snapshotLabel(s); got != tc.stamp {
			t.Fatalf("wrong SNAP label: %q", got)
		}
		if got := todayHeading(s); got != tc.heading {
			t.Fatalf("wrong TODAY date: %q", got)
		}
		for _, spec := range []struct {
			label string
			width int
			face  font.Face
		}{
			{snapshotLabel(s), hudWidth, r.small},
			{todayHeading(s), 670, r.normal},
		} {
			if width := font.MeasureString(r.faceFor(spec.label, spec.face), spec.label).Ceil(); width > spec.width {
				t.Fatalf("date label clips: %q width=%d max=%d", spec.label, width, spec.width)
			}
		}
	}
	if todayHeading(nil) != "TODAY / 活動" {
		t.Fatal("missing snapshot falsely labeled today")
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
