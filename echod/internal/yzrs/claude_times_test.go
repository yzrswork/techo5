package yzrs

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font"
)

func claudeFixture(captured time.Time, five float64) *Claude {
	seven := 59.0
	return &Claude{Source: "claude-statusline", Captured: captured,
		FiveHour: &Window{Remaining: &five, Reset: captured.Add(3 * time.Hour)},
		SevenDay: &Window{Remaining: &seven, Reset: captured.Add(72 * time.Hour)}}
}

func timesFixture(issue int) *Times {
	return &Times{Source: "yzrs-times", IssueNo: issue, Edition: "morning", Date: "2026-10-08",
		Fetched: fixtureTime, Title: "JSがこんなに減る！ HTML・CSS最新技術2026", Outlet: "はてなブックマーク テクノロジー"}
}

func TestClaudeTimesValidation(t *testing.T) {
	if !validClaude(claudeFixture(fixtureTime, 77), fixtureTime) || !validTimes(timesFixture(213), fixtureTime) {
		t.Fatal("valid fixtures rejected")
	}
	over := 101.0
	for name, c := range map[string]*Claude{
		"source": {Source: "token-monitor", Captured: fixtureTime, FiveHour: claudeFixture(fixtureTime, 1).FiveHour},
		"empty":  {Source: "claude-statusline", Captured: fixtureTime},
		"future": claudeFixture(fixtureTime.Add(time.Hour), 1),
		"range":  {Source: "claude-statusline", Captured: fixtureTime, FiveHour: &Window{Remaining: &over, Reset: fixtureTime}},
	} {
		if validClaude(c, fixtureTime) {
			t.Fatalf("invalid claude accepted: %s", name)
		}
	}
	for name, mutate := range map[string]func(*Times){
		"edition": func(x *Times) { x.Edition = "evening" },
		"issue":   func(x *Times) { x.IssueNo = 0 },
		"title":   func(x *Times) { x.Title = "" },
		"long":    func(x *Times) { x.Title = string(bytes.Repeat([]byte("あ"), 81)) },
		"date":    func(x *Times) { x.Date = "10/08" },
	} {
		x := timesFixture(1)
		mutate(x)
		if validTimes(x, fixtureTime) {
			t.Fatalf("invalid times accepted: %s", name)
		}
	}
	// The AI tab is LIVE only while the status line capture is recent.
	if !claudeLive(claudeFixture(fixtureTime, 77), fixtureTime.Add(15*time.Minute)) ||
		claudeLive(claudeFixture(fixtureTime, 77), fixtureTime.Add(16*time.Minute)) || claudeLive(nil, fixtureTime) {
		t.Fatal("claude LIVE window wrong")
	}
	// A passed reset means the window value is unknown, not still 77%.
	c := claudeFixture(fixtureTime, 77)
	if windowLeft(c.FiveHour, fixtureTime) == nil || windowLeft(c.FiveHour, fixtureTime.Add(3*time.Hour)) != nil {
		t.Fatal("window reset not honored")
	}
}

func TestClaudeTimesMonotonicAndWorkerCarryOver(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(fixture(t)) }))
	defer srv.Close()
	s, _ := NewStore("https://worker.invalid/dashboard", "test", filepath.Join(t.TempDir(), "cache.json"))
	s.URL, s.Client = srv.URL, srv.Client()
	if err := s.Fetch(context.Background(), fixtureTime); err != nil {
		t.Fatal(err)
	}
	now := fixtureTime.Add(time.Minute)
	if _, _, _, err := s.applyMetrics(Metrics{Claude: claudeFixture(fixtureTime, 77), Times: timesFixture(213)}, now, now); err != nil {
		t.Fatal(err)
	}
	// Older observations and lower issue numbers never replace newer values.
	s.applyMetrics(Metrics{Claude: claudeFixture(fixtureTime.Add(-time.Hour), 10), Times: timesFixture(212)}, now, now)
	v := s.View(now).Snapshot
	if *v.Claude.FiveHour.Remaining != 77 || v.Times.IssueNo != 213 {
		t.Fatal("regressed PC-only lane")
	}
	// A later Worker GET keeps both lanes, and the cache restores them.
	if err := s.Fetch(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	restored, _ := NewStore("https://worker.invalid/dashboard", "test", s.Cache)
	restored.Restore(now)
	for _, snap := range []*Snapshot{s.View(now).Snapshot, restored.View(now).Snapshot} {
		if snap.Claude == nil || snap.Times == nil || snap.Times.Title != timesFixture(1).Title {
			t.Fatal("PC-only lane lost")
		}
	}
	raw, _ := json.Marshal(Metrics{Claude: claudeFixture(fixtureTime, 77)})
	var c incomingCommand
	if json.Unmarshal(append([]byte(`{"op":"dashboard_result","id":"x",`), raw[1:]...), &c) != nil || c.Claude == nil {
		t.Fatal("dashboard_result does not decode claude lane")
	}
}

func TestClaudeTimesHUD(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	lines := wrapText(r.faceFor(timesFixture(1).Title, r.small), timesFixture(1).Title, hudWidth, 4)
	if len(lines) < 2 || len(lines) > 4 {
		t.Fatalf("unexpected wrap %q", lines)
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "ML") || strings.HasSuffix(line, "HT") {
			t.Fatalf("Latin word split across lines: %q", lines)
		}
	}
	for _, line := range lines[:len(lines)-1] {
		if w := font.MeasureString(r.faceFor(line, r.small), line).Ceil(); w > hudWidth {
			t.Fatalf("wrapped line overflows: %q", line)
		}
	}
	s, _ := Parse(fixture(t), fixtureTime)
	frame := Frame{Now: fixtureTime, Data: View{Snapshot: s, Status: "LIVE"}}
	for mode := range Modes {
		frame.Mode = mode
		s.Claude, s.Times = nil, nil
		before := image.NewRGBA(image.Rect(0, 0, 960, 480))
		r.Draw(before, frame)
		s.Claude, s.Times = claudeFixture(fixtureTime, 77), timesFixture(213)
		after := image.NewRGBA(before.Bounds())
		r.Draw(after, frame)
		changed := false
		for y := 0; y < 480; y++ {
			for x := 0; x < 960; x++ {
				if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
					changed = true
					inHUD := x >= hudLeft && x < hudLeft+hudWidth+4 && y >= 60 && y <= 400
					// The AI tab shows Claude in the content area; other tabs must not change.
					inAITab := mode == 2 && x >= 250 && y >= 60 && y <= 375
					if !inHUD && !inAITab {
						t.Fatalf("pixel outside Claude/Times areas changed: %d,%d mode=%d", x, y, mode)
					}
				}
			}
		}
		if !changed {
			t.Fatal("Claude/Times not rendered")
		}
	}
}

// Codex and host lanes are no longer displayed, so their absence must not
// report the PC as not updated; a missing displayed lane still does.
func TestRefreshStatusFollowsDisplayedLanes(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(fixture(t)) }))
	defer srv.Close()
	for _, tc := range []struct {
		m    Metrics
		want string
	}{
		{Metrics{Claude: claudeFixture(fixtureTime, 77), Times: timesFixture(213)}, "更新済"},
		{Metrics{Claude: claudeFixture(fixtureTime, 77)}, "PC未更新 / LKG"},
		{Metrics{Times: timesFixture(213)}, "PC未更新 / LKG"},
	} {
		s, _ := NewStore("https://worker.invalid/dashboard", "test", filepath.Join(t.TempDir(), "cache.json"))
		s.URL, s.Client = srv.URL, srv.Client()
		m := tc.m
		if !s.Refresh(context.Background(), func(context.Context) (Metrics, error) { return m, nil }) {
			t.Fatal("tap refused")
		}
		until := time.Now().Add(2 * time.Second)
		for s.RefreshStatus() == "更新中" && time.Now().Before(until) {
			time.Sleep(time.Millisecond)
		}
		if got := s.RefreshStatus(); got != tc.want {
			t.Fatalf("status %q, want %q", got, tc.want)
		}
	}
}
