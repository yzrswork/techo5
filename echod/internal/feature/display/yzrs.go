//go:build !dot && !spot

package display

import (
	"image"
	"sync"

	"github.com/HuskerMinion/techo5/echod/internal/feature/phone"
	feature "github.com/HuskerMinion/techo5/echod/internal/feature/yzrs"
	core "github.com/HuskerMinion/techo5/echod/internal/yzrs"
)

var yzrsRenderer struct {
	sync.Once
	r *core.Renderer
}

func showYZRS(s scene) bool {
	return feature.Get().Enabled() && s.phase == "idle" && !s.ring.any() && s.call.Phase == phone.Idle &&
		!s.showSheet && !s.showWifi && !s.showDrawer && !s.showCamera && !s.showDeck && !s.showDash &&
		!s.pin.open && !s.setupAsking && !s.bt.Pairing && !s.redClock && !s.showReminder &&
		!s.showAnnouncement && !s.announceRecording && s.popup == nil && !s.showAlert && !s.showCalendar &&
		!s.showWeather && !s.showRadar
}
func (r *renderer) drawYZRS(s scene) bool {
	if r.w != 960 || r.h != 480 || !showYZRS(s) {
		return false
	}
	yzrsRenderer.Do(func() { yzrsRenderer.r, _ = core.NewRenderer() })
	if yzrsRenderer.r == nil {
		return false
	}
	frame := feature.Get().Frame(s.now)
	frame.Weather = core.Weather{Condition: s.weather.Condition, Temp: s.weather.Temp, Updated: s.weather.Updated}
	yzrsRenderer.r.Draw(r.dst, frame)
	return true
}
func (d *Display) yzrsTap(x, y int) {
	if mode, ok := core.ModeAt(x, y); ok {
		feature.Get().Select(mode)
	}
	if image.Pt(x, y).In(image.Rect(846, 0, 960, 60)) {
		d.showSheet(true)
	}
	d.wake()
}
