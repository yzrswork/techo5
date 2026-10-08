package yzrs

import (
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed assets/MPLUS1p-Regular.ttf
var assets embed.FS
var Modes = [4]string{"CLOCK", "TODAY", "AI", "VOICE"}

type Renderer struct{ normal, small, large font.Face }

func NewRenderer() (*Renderer, error) {
	raw, e := assets.ReadFile("assets/MPLUS1p-Regular.ttf")
	if e != nil {
		return nil, e
	}
	f, e := opentype.Parse(raw)
	if e != nil {
		return nil, e
	}
	face := func(size float64) (font.Face, error) {
		return opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	}
	n, e := face(24)
	if e != nil {
		return nil, e
	}
	s, e := face(16)
	if e != nil {
		n.Close()
		return nil, e
	}
	l, e := face(112)
	if e != nil {
		n.Close()
		s.Close()
		return nil, e
	}
	return &Renderer{n, s, l}, nil
}
func (r *Renderer) Close()                { r.normal.Close(); r.small.Close(); r.large.Close() }
func (r *Renderer) HasGlyph(ch rune) bool { _, _, ok := r.normal.GlyphBounds(ch); return ok }

var navy = color.RGBA{7, 13, 32, 255}
var cyan = color.RGBA{82, 207, 244, 255}
var white = color.RGBA{223, 237, 255, 255}
var muted = color.RGBA{131, 157, 185, 255}

func rect(dst draw.Image, box image.Rectangle, c color.Color) {
	draw.Draw(dst, box, image.NewUniform(c), image.Point{}, draw.Src)
}
func (r *Renderer) text(dst draw.Image, x, y, width int, s string, f font.Face, c color.Color) {
	s = strings.Map(func(ch rune) rune {
		if ch < ' ' || ch == 127 {
			return ' '
		}
		if _, _, ok := f.GlyphBounds(ch); !ok {
			return '?'
		} // Unsupported symbols never produce missing-glyph boxes.
		return ch
	}, s)
	if font.MeasureString(f, s).Ceil() > width {
		chars := []rune(s)
		for len(chars) > 0 && font.MeasureString(f, string(chars)+"…").Ceil() > width {
			chars = chars[:len(chars)-1]
		}
		s = string(chars) + "…"
	}
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y)}
	d.DrawString(s)
}
func Count(n *int64) string {
	if n == nil {
		return "—"
	}
	return fmt.Sprint(*n)
}
func percent(n *float64) string {
	if n == nil {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", *n)
}

type Frame struct {
	Now   time.Time
	Mode  int
	Data  View
	Voice PTTView
	Deck  string
}

// Draw uses the same native raster path in the daemon and the PC screenshot tool.
// Target dimensions are deliberately fixed: the accepted Show 5 design is 960 × 480.
func (r *Renderer) Draw(dst draw.Image, f Frame) {
	rect(dst, image.Rect(0, 0, 960, 480), navy)
	// Subtle PCB traces stay behind content and away from the large clock.
	trace := color.RGBA{15, 43, 68, 255}
	for i := 0; i < 9; i++ {
		x := 245 + i*79
		rect(dst, image.Rect(x, 0, x+1, 40+i*4), trace)
		rect(dst, image.Rect(x, 40+i*4, x+25, 41+i*4), trace)
	}
	rect(dst, image.Rect(221, 24, 222, 455), trace)
	now := f.Now.In(JST)
	r.text(dst, 24, 40, 175, "YZRS / TECHO5", r.small, cyan)
	r.text(dst, 250, 43, 570, now.Format("2006年01月02日")+"  "+[]string{"日", "月", "火", "水", "木", "金", "土"}[now.Weekday()]+"曜日", r.normal, muted)
	r.text(dst, 856, 42, 86, "SETUP", r.small, muted)
	var tokens, commits, notes *int64
	var remaining *float64
	if s := f.Data.Snapshot; s != nil {
		commits = s.Today.Commits
		notes = s.Today.Notes
		if s.AI != nil {
			tokens = s.AI.Codex.Tokens
			remaining = s.AI.Codex.Session.Remaining
		}
	}
	metrics := [][2]string{{"TOKENS TODAY", Count(tokens)}, {"LIMIT REMAINING", percent(remaining)}, {"CODEX TEMP", "UNAVAILABLE"}, {"COMMITS", Count(commits)}, {"NOTES", Count(notes)}}
	for i, m := range metrics {
		y := 80 + i*64
		r.text(dst, 24, y, 179, m[0], r.small, muted)
		r.text(dst, 24, y+28, 179, m[1], r.normal, white)
	}
	r.text(dst, 24, 422, 183, f.Data.Status, r.small, cyan)
	if f.Data.Snapshot != nil {
		r.text(dst, 24, 445, 183, "SYNC "+f.Data.Snapshot.Generated.In(JST).Format("15:04"), r.small, muted)
	}
	mode := f.Mode
	if mode < 0 || mode > 3 {
		mode = 0
	}
	switch mode {
	case 0:
		r.text(dst, 267, 237, 668, now.Format("15:04"), r.large, white)
		r.text(dst, 272, 286, 640, "常設時計 / PERSONAL DASHBOARD", r.normal, cyan)
	case 1:
		r.text(dst, 256, 100, 670, "TODAY / 今日の活動", r.normal, cyan)
		s := f.Data.Snapshot
		if s == nil {
			r.text(dst, 256, 157, 665, "データを取得できません。接続を待っています。", r.normal, muted)
		} else {
			r.text(dst, 256, 145, 665, "NOTES +"+Count(s.Today.NewNotes)+"   COMMITS "+Count(s.Today.Commits), r.normal, white)
			if len(s.Activity) == 0 {
				r.text(dst, 256, 195, 665, "今日の活動はまだありません。", r.normal, muted)
			}
			for i, a := range s.Activity {
				if i >= 4 {
					break
				}
				r.text(dst, 256, 197+i*46, 665, a.At.In(JST).Format("15:04")+"  "+a.Title, r.normal, white)
			}
			if len(s.Activity) > 4 {
				r.text(dst, 256, 386, 665, fmt.Sprintf("ほか %d 件", len(s.Activity)-4), r.small, muted)
			}
		}
	case 2:
		r.text(dst, 256, 100, 665, "AI / CODEX", r.normal, cyan)
		s := f.Data.Snapshot
		if s == nil || s.AI == nil {
			r.text(dst, 256, 161, 665, "AI データは利用できません。", r.normal, muted)
		} else {
			a := s.AI
			status := "STALE"
			if a.Live(now) {
				status = "LIVE"
			}
			r.text(dst, 256, 153, 665, a.Codex.Plan+"  /  "+status, r.normal, white)
			r.text(dst, 256, 205, 665, "SESSION REMAINING  "+percent(a.Codex.Session.Remaining), r.normal, white)
			r.text(dst, 256, 235, 665, "RESET "+a.Codex.Session.Reset.In(JST).Format("01/02 15:04"), r.small, muted)
			r.text(dst, 256, 285, 665, "WEEKLY REMAINING   "+percent(a.Codex.Weekly.Remaining), r.normal, white)
			r.text(dst, 256, 315, 665, "RESET "+a.Codex.Weekly.Reset.In(JST).Format("01/02 15:04"), r.small, muted)
			r.text(dst, 256, 367, 665, "UPDATED "+a.Updated.In(JST).Format("01/02 15:04"), r.small, muted)
		}
	case 3:
		r.text(dst, 256, 100, 665, "VOICE / WINDOWS PTT", r.normal, cyan)
		phase := f.Voice.Phase
		if phase == "" {
			phase = "IDLE"
		}
		r.text(dst, 256, 165, 665, phase, r.normal, white)
		status := "PC CLIENT UNAVAILABLE"
		if f.Voice.Connected {
			status = "PC CLIENT ACTIVE"
		}
		r.text(dst, 256, 215, 665, status, r.normal, muted)
		r.text(dst, 256, 260, 665, "F8 を押して話す / 離して文字起こし", r.normal, white)
		r.text(dst, 256, 310, 665, "VOICE は Windows クライアントの起動操作です。", r.normal, muted)
		r.text(dst, 256, 360, 665, "DECK / "+f.Deck, r.small, cyan)
	}
	if s := f.Data.Snapshot; s != nil && s.AI != nil && !s.AI.Live(now) {
		r.text(dst, 24, 464, 184, "AI STALE", r.small, muted)
	}
	for i, name := range Modes {
		box := image.Rect(252+i*171, 413, 410+i*171, 465)
		c := color.RGBA{20, 30, 53, 255}
		if i == mode {
			c = color.RGBA{24, 61, 85, 255}
		}
		// Rounded rectangles using a small scanline inset, no heavyweight graphics runtime.
		for y := box.Min.Y; y < box.Max.Y; y++ {
			inset := 0
			dy := min(y-box.Min.Y, box.Max.Y-1-y)
			if dy < 5 {
				inset = 5 - dy
			}
			rect(dst, image.Rect(box.Min.X+inset, y, box.Max.X-inset, y+1), c)
		}
		r.text(dst, box.Min.X+28, 446, 126, name, r.normal, white)
	}
}
func ModeAt(x, y int) (int, bool) {
	if y < 413 || y >= 465 {
		return 0, false
	}
	for i := range Modes {
		if x >= 252+i*171 && x < 410+i*171 {
			return i, true
		}
	}
	return 0, false
}
