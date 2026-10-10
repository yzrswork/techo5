package yzrs

import (
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed assets/MPLUS1p-Regular.ttf assets/ChakraPetch-Regular.ttf
var assets embed.FS
var Modes = [4]string{"CLOCK", "TODAY", "AI", "VOICE"}

const hudLeft, hudWidth = 36, 176

type Renderer struct {
	normal, small, large, latin, latinSmall font.Face
	background                              *image.RGBA
}

func NewRenderer() (*Renderer, error) {
	raw, e := assets.ReadFile("assets/MPLUS1p-Regular.ttf")
	if e != nil {
		return nil, e
	}
	f, e := opentype.Parse(raw)
	if e != nil {
		return nil, e
	}
	rawLatin, e := assets.ReadFile("assets/ChakraPetch-Regular.ttf")
	if e != nil {
		return nil, e
	}
	geometric, e := opentype.Parse(rawLatin)
	if e != nil {
		return nil, e
	}
	r := &Renderer{background: newBackground()}
	for _, spec := range []struct {
		source *opentype.Font
		size   float64
		target *font.Face
	}{
		{f, 24, &r.normal}, {f, 16, &r.small}, {geometric, 112, &r.large}, {geometric, 24, &r.latin}, {geometric, 16, &r.latinSmall},
	} {
		face, err := opentype.NewFace(spec.source, &opentype.FaceOptions{Size: spec.size, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			r.Close()
			return nil, err
		}
		*spec.target = face
	}
	return r, nil
}
func (r *Renderer) Close() {
	for _, face := range []font.Face{r.normal, r.small, r.large, r.latin, r.latinSmall} {
		if face != nil {
			face.Close()
		}
	}
}

// Keep complete mixed/Japanese strings on the accepted Japanese face.
func (r *Renderer) faceFor(text string, face font.Face) font.Face {
	for _, ch := range text {
		if ch < 32 || ch > 126 {
			return face
		}
	}
	if face == r.normal {
		return r.latin
	}
	if face == r.small {
		return r.latinSmall
	}
	return face
}
func (r *Renderer) HasGlyph(ch rune) bool { _, _, ok := r.normal.GlyphBounds(ch); return ok }

var navy = color.RGBA{8, 16, 35, 255}
var indigo = color.RGBA{23, 20, 61, 255}
var cyan = color.RGBA{82, 207, 244, 255}
var white = color.RGBA{223, 237, 255, 255}
var muted = color.RGBA{131, 157, 185, 255}

func blendColor(a, b color.RGBA, weight int) color.RGBA {
	return color.RGBA{
		R: uint8((int(a.R)*(100-weight) + int(b.R)*weight + 50) / 100),
		G: uint8((int(a.G)*(100-weight) + int(b.G)*weight + 50) / 100),
		B: uint8((int(a.B)*(100-weight) + int(b.B)*weight + 50) / 100),
		A: 255,
	}
}

func hashNoise(x, y int) float64 {
	n := uint32(x)*0x1f123bb5 ^ uint32(y)*0x5f356495 ^ 0x6c8e9cf5
	n ^= n >> 16
	n *= 0x7feb352d
	n ^= n >> 15
	n *= 0x846ca68b
	n ^= n >> 16
	return float64(n) / float64(^uint32(0))
}

func smoothNoise(x, y float64) float64 {
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	tx, ty := x-float64(x0), y-float64(y0)
	tx = tx * tx * (3 - 2*tx)
	ty = ty * ty * (3 - 2*ty)
	lerp := func(a, b, t float64) float64 { return a + (b-a)*t }
	a := lerp(hashNoise(x0, y0), hashNoise(x0+1, y0), tx)
	b := lerp(hashNoise(x0, y0+1), hashNoise(x0+1, y0+1), tx)
	return lerp(a, b, ty)
}

func overPixel(dst color.RGBA, src color.RGBA, alpha float64) color.RGBA {
	if alpha <= 0 {
		return dst
	}
	if alpha > 1 {
		alpha = 1
	}
	keep := 1 - alpha
	return color.RGBA{
		R: uint8(float64(dst.R)*keep + float64(src.R)*alpha + 0.5),
		G: uint8(float64(dst.G)*keep + float64(src.G)*alpha + 0.5),
		B: uint8(float64(dst.B)*keep + float64(src.B)*alpha + 0.5),
		A: 255,
	}
}

func paintGlow(dst *image.RGBA, cx, cy, radius int, c color.RGBA, strength float64) {
	for y := max(0, cy-radius); y <= min(dst.Bounds().Max.Y-1, cy+radius); y++ {
		for x := max(0, cx-radius); x <= min(dst.Bounds().Max.X-1, cx+radius); x++ {
			dx, dy := float64(x-cx), float64(y-cy)
			d := (dx*dx + dy*dy) / float64(radius*radius)
			if d < 1 {
				p := dst.RGBAAt(x, y)
				dst.SetRGBA(x, y, overPixel(p, c, strength*math.Exp(-5*d)))
			}
		}
	}
}

// newBackground recreates only the reference's Midnight Navy starfield, blue
// nebula, and perimeter circuitry. All dashboard content remains native text.
func newBackground() *image.RGBA {
	const width, height = 960, 480
	background := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			u, v := float64(x)/float64(width), float64(y)/float64(height)
			weight := int(5 + 12*u + 9*v)
			base := blendColor(navy, indigo, weight)

			// A broken diagonal cloud runs through the center-right, leaving the
			// native HUD and text legible over a dark, low-contrast field.
			lineY := 400 - 0.37*float64(x) + 13*math.Sin(float64(x)/43) + 7*math.Sin(float64(x)/19)
			cross := (float64(y) - lineY) / 77
			longitudinal := math.Exp(-math.Pow((float64(x)-650)/390, 4))
			broad := math.Exp(-0.5 * cross * cross)
			noise := 0.52*smoothNoise(float64(x)/88, float64(y)/70) +
				0.31*smoothNoise(float64(x)/31, float64(y)/27) +
				0.17*smoothNoise(float64(x)/11, float64(y)/9)
			filaments := 0.20 + 0.80*noise
			alpha := broad * longitudinal * filaments * 0.62
			base = overPixel(base, color.RGBA{R: 34, G: 86, B: 196, A: 255}, alpha)
			inner := math.Exp(-0.5*math.Pow(cross/0.39, 2)) * longitudinal
			base = overPixel(base, color.RGBA{R: 67, G: 137, B: 255, A: 255}, inner*noise*0.34)

			// A faint blue-violet halo adds depth without introducing any labels
			// or values from the complete mockup.
			dx, dy := (float64(x)-654)/202, (float64(y)-225)/119
			halo := math.Exp(-0.5 * (dx*dx + dy*dy))
			base = overPixel(base, color.RGBA{R: 44, G: 52, B: 139, A: 255}, halo*0.14)

			// Keep the extreme corners dark so the circuit traces frame the UI.
			edge := math.Max(math.Abs(u-0.5)*2, math.Abs(v-0.5)*2)
			vignette := math.Max(0, (edge-0.62)/0.38) * 0.24
			base = overPixel(base, color.RGBA{R: 2, G: 6, B: 17, A: 255}, vignette)
			background.SetRGBA(x, y, base)
		}
	}

	// Fine stars are deterministic so the device background does not shimmer
	// or change between frames.
	rng := rand.New(rand.NewSource(0x5a17f13))
	for i := 0; i < 510; i++ {
		x, y := rng.Intn(width), rng.Intn(height)
		alpha := 0.18 + rng.Float64()*0.40
		star := color.RGBA{R: 126, G: 174, B: 255, A: 255}
		if i%11 == 0 {
			star = color.RGBA{R: 204, G: 229, B: 255, A: 255}
			paintGlow(background, x, y, 3, star, alpha*0.38)
		}
		p := background.RGBAAt(x, y)
		background.SetRGBA(x, y, overPixel(p, star, alpha))
	}

	trace := image.NewUniform(color.NRGBA{R: 38, G: 152, B: 255, A: 94})
	node := image.NewUniform(color.NRGBA{R: 91, G: 204, B: 255, A: 160})
	paint := func(box image.Rectangle, brush image.Image) {
		draw.Draw(background, box, brush, image.Point{}, draw.Over)
	}
	// Layered orthogonal runs and glowing vias frame the outer perimeter.
	for _, x := range []int{31, 57, 83, 112, 143} {
		y := 10 + (x%4)*4
		paint(image.Rect(x, 0, x+2, y+12), trace)
		paint(image.Rect(x, y+10, x+34, y+12), trace)
		paint(image.Rect(x+32, y+10, x+34, y+26), trace)
		paint(image.Rect(x+32, y+24, x+51, y+26), trace)
		paint(image.Rect(x+48, y+21, x+54, y+27), node)
	}
	for _, x := range []int{686, 728, 774, 821} {
		y := 7 + (x%3)*5
		paint(image.Rect(x, 0, x+2, y+18), trace)
		paint(image.Rect(x, y+16, x+24, y+18), trace)
		paint(image.Rect(x+22, y+16, x+24, y+33), trace)
		paint(image.Rect(x+21, y+31, x+41, y+33), trace)
		paint(image.Rect(x+38, y+28, x+44, y+34), node)
	}
	// Outer side runs stay in the margins; short branches stop before labels.
	for _, x := range []int{8, 16, 24, 936, 944, 952} {
		paint(image.Rect(x, 0, x+1, height), trace)
	}
	for _, y := range []int{58, 117, 174, 236, 296, 357, 418} {
		paint(image.Rect(0, y, 17+(y%3)*5, y+1), trace)
		paint(image.Rect(943, y, 960, y+1), trace)
		paint(image.Rect(12, y-3, 18, y+3), node)
		paint(image.Rect(940, y-3, 946, y+3), node)
	}
	// Fine horizontal runs occupy the bottom border below the touch targets.
	for _, x := range []int{34, 83, 146, 196, 742, 802, 868} {
		y := 472 - (x%3)*3
		paint(image.Rect(x, y, x+2, height), trace)
		paint(image.Rect(x, y, x+25, y+2), trace)
		paint(image.Rect(x+23, y-13, x+25, y+2), trace)
		paint(image.Rect(x+22, y-15, x+28, y-9), node)
	}
	// Quiet inner rails echo the reference's circuit-panel framing.
	paint(image.Rect(221, 48, 223, 405), trace)
	for y := 88; y <= 376; y += 96 {
		paint(image.Rect(221, y, 240, y+2), trace)
		paint(image.Rect(237, y-2, 241, y+4), node)
	}
	paint(image.Rect(929, 68, 931, 392), trace)
	for y := 112; y <= 352; y += 80 {
		paint(image.Rect(929, y, 945, y+2), trace)
		paint(image.Rect(927, y-2, 933, y+4), node)
	}
	// Soften only the content strip once, retaining perimeter PCB and the main starfield.
	for y := 22; y < 470; y++ {
		for x := 28; x < 220; x++ {
			edge := min(x-28, 219-x, y-22, 469-y)
			alpha := 0.66 * math.Min(1, float64(edge)/12)
			background.SetRGBA(x, y, overPixel(background.RGBAAt(x, y), navy, alpha))
		}
	}
	return background
}

func rect(dst draw.Image, box image.Rectangle, c color.Color) {
	draw.Draw(dst, box, image.NewUniform(c), image.Point{}, draw.Src)
}
func (r *Renderer) text(dst draw.Image, x, y, width int, s string, f font.Face, c color.Color) {
	f = r.faceFor(s, f)
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
	v := fmt.Sprint(*n)
	start := 0
	if strings.HasPrefix(v, "-") {
		start = 1
	}
	for i := len(v) - 3; i > start; i -= 3 {
		v = v[:i] + "," + v[i:]
	}
	return v
}
func percent(n *float64) string {
	if n == nil {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", *n)
}

func aiStatus(a *AI, now time.Time, dataStatus string) string {
	if dataStatus == "OFFLINE / LKG" {
		return "OFFLINE / LKG"
	}
	if a.Live(now) {
		return "LIVE"
	}
	return "STALE"
}

type Frame struct {
	Now     time.Time
	Mode    int
	Data    View
	Voice   PTTView
	Deck    string
	Weather Weather
}

// Draw uses the same native raster path in the daemon and the PC screenshot tool.
// Target dimensions are deliberately fixed: the accepted Show 5 design is 960 × 480.
func (r *Renderer) Draw(dst draw.Image, f Frame) {
	draw.Draw(dst, image.Rect(0, 0, 960, 480), r.background, image.Point{}, draw.Src)
	now := f.Now.In(JST)
	r.text(dst, hudLeft, 40, hudWidth, "YZRS / TECHO5", r.small, cyan)
	r.text(dst, 256, 42, 560, "PERSONAL DASHBOARD", r.small, muted)
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
	r.text(dst, hudLeft, 77, hudWidth, "TOKENS TODAY", r.small, muted)
	r.text(dst, hudLeft, 105, hudWidth, Count(tokens), r.normal, white)
	r.text(dst, hudLeft, 140, hudWidth, "LIMIT REMAINING", r.small, muted)
	gaugeColor := cyan
	gaugeStatus := "UNAVAILABLE"
	if f.Data.Snapshot != nil && f.Data.Snapshot.AI != nil {
		gaugeStatus = aiStatus(f.Data.Snapshot.AI, now, f.Data.Status)
		if gaugeStatus != "LIVE" {
			gaugeColor = muted
		}
	}
	drawRing(dst, hudLeft+hudWidth/2, 200, 43, 5, remaining, gaugeColor)
	r.center(dst, hudLeft, 198, hudWidth, percent(remaining), r.normal, white)
	r.center(dst, hudLeft, 256, hudWidth, gaugeStatus, r.small, muted)
	for i, m := range [][2]string{{"CODEX TEMP", "UNAVAILABLE"}, {"COMMITS", Count(commits)}, {"NOTES", Count(notes)}} {
		y := 273 + i*48
		r.text(dst, hudLeft, y, hudWidth, m[0], r.small, muted)
		r.text(dst, hudLeft, y+24, hudWidth, m[1], r.normal, white)
	}
	r.text(dst, hudLeft, 422, hudWidth, f.Data.Status, r.small, cyan)
	if f.Data.Snapshot != nil {
		r.text(dst, hudLeft, 445, hudWidth, "SYNC "+f.Data.Snapshot.Generated.In(JST).Format("15:04"), r.small, muted)
	}
	mode := f.Mode
	if mode < 0 || mode > 3 {
		mode = 0
	}
	switch mode {
	case 0:
		r.center(dst, 250, 218, 676, now.Format("15:04"), r.large, white)
		r.center(dst, 250, 269, 676, now.Format("2006年01月02日")+"  "+[]string{"日", "月", "火", "水", "木", "金", "土"}[now.Weekday()]+"曜日", r.normal, muted)
		r.drawWeather(dst, f.Weather, now)
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
				if i >= 3 {
					break
				}
				y := 190 + i*56
				r.text(dst, 256, y, 72, a.At.In(JST).Format("15:04"), r.small, cyan)
				lines := wrapTitle(a.Title, r.normal, 582)
				for j, line := range lines {
					r.text(dst, 339, y+j*26, 582, line, r.normal, white)
				}
			}
			if len(s.Activity) > 3 {
				r.text(dst, 256, 364, 665, fmt.Sprintf("ほか %d 件", len(s.Activity)-3), r.small, muted)
			}
		}
	case 2:
		r.text(dst, 256, 100, 665, "AI / CODEX", r.normal, cyan)
		s := f.Data.Snapshot
		if s == nil || s.AI == nil {
			r.text(dst, 256, 161, 665, "AI データは利用できません。", r.normal, muted)
		} else {
			a := s.AI
			status := aiStatus(a, now, f.Data.Status)
			r.text(dst, 256, 153, 665, a.Codex.Plan+"  /  "+status, r.normal, white)
			r.text(dst, 256, 205, 665, "SESSION REMAINING  "+percent(a.Codex.Session.Remaining), r.normal, white)
			r.text(dst, 256, 235, 665, "RESET "+a.Codex.Session.Reset.In(JST).Format("01/02 15:04"), r.small, muted)
			r.text(dst, 256, 285, 665, "WEEKLY REMAINING   "+percent(a.Codex.Weekly.Remaining), r.normal, white)
			r.text(dst, 256, 315, 665, "RESET "+a.Codex.Weekly.Reset.In(JST).Format("01/02 15:04"), r.small, muted)
			r.text(dst, 256, 362, 665, "UPDATED "+a.Updated.In(JST).Format("01/02 15:04"), r.small, muted)
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
		r.text(dst, 256, 310, 665, "VOICE は Windows クライアントを起動します。", r.normal, muted)
	}
	if s := f.Data.Snapshot; s != nil && s.AI != nil && !s.AI.Live(now) {
		r.text(dst, hudLeft, 464, hudWidth, "AI STALE", r.small, muted)
	}
	for i, name := range Modes {
		box := NavBounds(i)
		border := color.RGBA{34, 58, 88, 255}
		if i == mode {
			roundedRect(dst, box.Inset(-2), 14, color.RGBA{17, 63, 86, 255})
			border = cyan
		}
		roundedRect(dst, box, 12, border)
		roundedRect(dst, box.Inset(1), 11, color.RGBA{13, 24, 46, 255})
		iconColor := muted
		if i == mode {
			iconColor = cyan
		}
		drawNavIcon(dst, i, (box.Min.X+box.Max.X)/2, box.Min.Y+29, iconColor)
		r.center(dst, box.Min.X, box.Min.Y+69, box.Dx(), name, r.small, white)
	}
}

// One rectangle authority keeps visible tiles and touch targets identical.
func NavBounds(mode int) image.Rectangle { return image.Rect(346+mode*124, 380, 456+mode*124, 466) }
func ModeAt(x, y int) (int, bool) {
	for i := range Modes {
		if image.Pt(x, y).In(NavBounds(i)) {
			return i, true
		}
	}
	return 0, false
}
