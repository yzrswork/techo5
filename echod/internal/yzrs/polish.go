package yzrs

import (
	"fmt"
	"golang.org/x/image/font"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"time"
)

type Weather struct {
	Condition, Temp string
	Updated         time.Time
}

func (r *Renderer) center(dst draw.Image, x, y, width int, text string, face font.Face, c color.Color) {
	r.text(dst, x+max(0, (width-font.MeasureString(face, text).Ceil())/2), y, width, text, face, c)
}

func roundedRect(dst draw.Image, b image.Rectangle, radius int, c color.Color) {
	for y := b.Min.Y; y < b.Max.Y; y++ {
		edge := min(y-b.Min.Y, b.Max.Y-1-y)
		inset := 0
		if edge < radius {
			d := float64(radius - edge)
			inset = radius - int(math.Sqrt(float64(radius*radius)-d*d))
		}
		rect(dst, image.Rect(b.Min.X+inset, y, b.Max.X-inset, y+1), c)
	}
}

func drawRing(dst draw.Image, cx, cy, radius, thickness int, remaining *float64, c color.Color) {
	valid := remaining != nil && !math.IsNaN(*remaining) && !math.IsInf(*remaining, 0) && *remaining >= 0 && *remaining <= 100
	for y := cy - radius; y <= cy+radius; y++ {
		for x := cx - radius; x <= cx+radius; x++ {
			dx, dy := float64(x-cx), float64(y-cy)
			d := math.Hypot(dx, dy)
			if d < float64(radius-thickness) || d > float64(radius) {
				continue
			}
			angle := math.Mod(math.Atan2(dy, dx)+math.Pi/2+2*math.Pi, 2*math.Pi)
			ink := color.Color(color.RGBA{34, 53, 78, 255})
			if valid && angle < 2*math.Pi*(*remaining)/100 {
				ink = c
			}
			dst.Set(x, y, ink)
		}
	}
}

func line(dst draw.Image, x0, y0, x1, y1 int, c color.Color) {
	steps := max(abs(x1-x0), abs(y1-y0))
	if steps == 0 {
		dst.Set(x0, y0, c)
		return
	}
	for i := 0; i <= steps; i++ {
		x := x0 + (x1-x0)*i/steps
		y := y0 + (y1-y0)*i/steps
		rect(dst, image.Rect(x, y, x+2, y+2), c)
	}
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func drawNavIcon(dst draw.Image, mode, cx, cy int, c color.Color) {
	switch mode {
	case 0:
		full := 100.0
		drawRing(dst, cx, cy, 16, 2, &full, c)
		line(dst, cx, cy, cx, cy-9, c)
		line(dst, cx, cy, cx+8, cy+4, c)
	case 1:
		roundedRect(dst, image.Rect(cx-15, cy-15, cx+16, cy+16), 4, c)
		roundedRect(dst, image.Rect(cx-13, cy-8, cx+14, cy+14), 2, navy)
		for _, dy := range []int{-2, 5, 11} {
			line(dst, cx-8, cy+dy, cx+8, cy+dy, c)
		}
	case 2:
		roundedRect(dst, image.Rect(cx-11, cy-11, cx+12, cy+12), 3, c)
		roundedRect(dst, image.Rect(cx-9, cy-9, cx+10, cy+10), 2, navy)
		for _, d := range []int{-7, 0, 7} {
			line(dst, cx+d, cy-17, cx+d, cy-12, c)
			line(dst, cx+d, cy+12, cx+d, cy+17, c)
			line(dst, cx-17, cy+d, cx-12, cy+d, c)
			line(dst, cx+12, cy+d, cx+17, cy+d, c)
		}
	case 3:
		roundedRect(dst, image.Rect(cx-6, cy-17, cx+7, cy+7), 6, c)
		line(dst, cx-12, cy, cx-12, cy+8, c)
		line(dst, cx+12, cy, cx+12, cy+8, c)
		line(dst, cx-12, cy+8, cx, cy+14, c)
		line(dst, cx, cy+14, cx+12, cy+8, c)
		line(dst, cx, cy+14, cx, cy+19, c)
	}
}

// Keep the time in its own column; preserve source text and ellipsize only line two.
func wrapTitle(text string, face font.Face, width int) []string {
	text = strings.Map(func(ch rune) rune {
		if ch < ' ' || ch == 127 {
			return ' '
		}
		return ch
	}, text)
	chars := []rune(text)
	cut := 0
	for cut < len(chars) && font.MeasureString(face, string(chars[:cut+1])).Ceil() <= width {
		cut++
	}
	if cut == len(chars) {
		return []string{text}
	}
	// Avoid starting line two with Japanese closing punctuation where possible.
	for cut > 1 && strings.ContainsRune("、。）」』】！？", chars[cut]) {
		cut--
	}
	return []string{string(chars[:cut]), string(chars[cut:])}
}

func weatherLabel(condition string) string {
	switch condition {
	case "sunny":
		return "晴れ"
	case "clear-night":
		return "晴れ（夜）"
	case "partlycloudy":
		return "晴れ時々曇り"
	case "cloudy":
		return "曇り"
	case "rainy", "rain":
		return "雨"
	case "pouring":
		return "強い雨"
	case "snowy", "snow":
		return "雪"
	case "snowy-rainy":
		return "みぞれ"
	case "fog":
		return "霧"
	case "lightning", "lightning-rainy":
		return "雷雨"
	case "windy", "windy-variant":
		return "風が強い"
	case "hail":
		return "ひょう"
	default:
		return "天気情報なし"
	}
}

func (r *Renderer) drawWeather(dst draw.Image, w Weather, now time.Time) {
	label := weatherLabel(w.Condition)
	if w.Condition == "" || w.Temp == "" || (!w.Updated.IsZero() && (now.Sub(w.Updated) > 3*time.Hour || w.Updated.After(now.Add(5*time.Minute)))) {
		r.center(dst, 250, 319, 676, "天気情報を取得できません", r.normal, muted)
		return
	}
	text := w.Temp + "  " + label
	if !w.Updated.IsZero() && now.Sub(w.Updated) > 45*time.Minute {
		text += fmt.Sprintf(" / %d分前", int(now.Sub(w.Updated).Minutes()))
	}
	width := font.MeasureString(r.normal, text).Ceil() + 58
	x := 250 + (676-width)/2
	// Small native weather mark, without depending on symbol-font coverage.
	cx, cy := x+18, 310
	if w.Condition == "sunny" || w.Condition == "clear-night" {
		full := 100.0
		drawRing(dst, cx, cy, 9, 2, &full, cyan)
		for i := 0; i < 8; i++ {
			a := float64(i) * math.Pi / 4
			line(dst, cx+int(13*math.Cos(a)), cy+int(13*math.Sin(a)), cx+int(17*math.Cos(a)), cy+int(17*math.Sin(a)), cyan)
		}
	} else {
		roundedRect(dst, image.Rect(cx-17, cy-4, cx+18, cy+9), 6, cyan)
		roundedRect(dst, image.Rect(cx-10, cy-12, cx+9, cy+9), 8, cyan)
		if strings.Contains(w.Condition, "rain") || w.Condition == "pouring" {
			for _, dx := range []int{-10, 0, 10} {
				line(dst, cx+dx, cy+13, cx+dx-3, cy+18, cyan)
			}
		}
	}
	r.text(dst, x+48, 319, 600, text, r.normal, white)
}
