package yzrs

import (
	"fmt"
	"image/draw"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/image/font"
)

// Claude is the Claude Code status line rate-limit snapshot written on the PC.
// It is captured whenever Claude Code runs, so it can be older than the refresh.
type Claude struct {
	Source   string    `json:"source"`
	Captured time.Time `json:"capturedAt"`
	FiveHour *Window   `json:"fiveHour,omitempty"`
	SevenDay *Window   `json:"sevenDay,omitempty"`
}

// Times is the lead article of the latest public YZRS Times issue.
type Times struct {
	Source  string    `json:"source"`
	IssueNo int       `json:"issueNo"`
	Edition string    `json:"edition"`
	Date    string    `json:"date"`
	Fetched time.Time `json:"fetchedAt"`
	Title   string    `json:"title"`
	Outlet  string    `json:"outlet"`
}

var timesDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func validWindow(w *Window) bool {
	return w == nil || (w.Remaining != nil && *w.Remaining >= 0 && *w.Remaining <= 100 && !w.Reset.IsZero())
}

func validClaude(c *Claude, now time.Time) bool {
	return c != nil && c.Source == "claude-statusline" && !c.Captured.IsZero() && !c.Captured.After(now.Add(5*time.Minute)) &&
		(c.FiveHour != nil || c.SevenDay != nil) && validWindow(c.FiveHour) && validWindow(c.SevenDay)
}

func validTimes(t *Times, now time.Time) bool {
	return t != nil && t.Source == "yzrs-times" && t.IssueNo > 0 && (t.Edition == "morning" || t.Edition == "midday") &&
		timesDate.MatchString(t.Date) && !t.Fetched.IsZero() && !t.Fetched.After(now.Add(5*time.Minute)) &&
		t.Title != "" && utf8.RuneCountInString(t.Title) <= 80 && utf8.RuneCountInString(t.Outlet) <= 40
}

// windowLeft is the remaining percentage while the window is current. Claude Code
// drops a window after resets_at, so a passed reset means the value is unknown.
func windowLeft(w *Window, now time.Time) *float64 {
	if w == nil || w.Remaining == nil || !now.Before(w.Reset) {
		return nil
	}
	return w.Remaining
}

// drawClaudeHUD shows the 5-hour window as the ring and the 7-day window below it.
func (r *Renderer) drawClaudeHUD(dst draw.Image, c *Claude, now time.Time) {
	r.text(dst, hudLeft, 77, hudWidth, "CLAUDE 5H LEFT", r.small, muted)
	var five, seven *float64
	reset := "NO DATA"
	if c != nil {
		five, seven = windowLeft(c.FiveHour, now), windowLeft(c.SevenDay, now)
		reset = "WINDOW RESET"
		if five != nil {
			reset = "RESET " + c.FiveHour.Reset.In(JST).Format("15:04")
		}
	}
	ring := cyan
	if five == nil {
		ring = muted
	}
	drawRing(dst, hudLeft+hudWidth/2, 132, 34, 5, five, ring)
	r.center(dst, hudLeft, 139, hudWidth, percent(five), r.normal, white)
	r.center(dst, hudLeft, 192, hudWidth, reset, r.small, muted)
	r.text(dst, hudLeft, 216, hudWidth, "7D LEFT "+percent(seven), r.small, white)
}

// drawTimesHUD shows the lead article of the latest YZRS Times issue.
func (r *Renderer) drawTimesHUD(dst draw.Image, t *Times) {
	if t == nil {
		r.text(dst, hudLeft, 254, hudWidth, "YZRS TIMES", r.small, cyan)
		r.text(dst, hudLeft, 278, hudWidth, "NO DATA", r.small, muted)
		return
	}
	edition := "朝刊"
	if t.Edition == "midday" {
		edition = "昼刊"
	}
	r.text(dst, hudLeft, 254, hudWidth, fmt.Sprintf("YZRS TIMES %d %s", t.IssueNo, edition), r.small, cyan)
	lines := wrapText(r.faceFor(t.Title, r.small), t.Title, hudWidth, 4)
	for i, line := range lines {
		r.text(dst, hudLeft, 280+i*22, hudWidth, line, r.small, white)
	}
	r.text(dst, hudLeft, 284+len(lines)*22, hudWidth, t.Outlet, r.small, muted)
}

func latinWord(ch rune) bool {
	return ch < 128 && (ch >= '0' && ch <= '9' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch == '.' || ch == '-')
}

// wrapText breaks by measured width per rune (Japanese has no spaces) but keeps
// Latin words such as "HTML" whole when they fit on a line. The final line keeps
// the remainder; Renderer.text truncates it with an ellipsis.
func wrapText(f font.Face, s string, width, maxLines int) []string {
	var lines []string
	line := []rune{}
	for _, ch := range []rune(s) {
		if len(line) == 0 && ch == ' ' {
			continue
		}
		if len(lines) < maxLines-1 && len(line) > 0 && font.MeasureString(f, string(append(line, ch))).Ceil() > width {
			cut := len(line)
			if latinWord(ch) {
				for cut > 0 && latinWord(line[cut-1]) {
					cut--
				}
				if cut == 0 {
					cut = len(line) // a word longer than the line still breaks
				}
			}
			lines = append(lines, strings.TrimRight(string(line[:cut]), " "))
			line = append(line[:0:0], line[cut:]...)
			if ch == ' ' {
				continue
			}
		}
		line = append(line, ch)
	}
	if len(line) > 0 {
		lines = append(lines, string(line))
	}
	return lines
}
