package yzrs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIdle24HourModelNoDashboardRequests(t *testing.T) {
	var gets atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gets.Add(1); w.Write(fixture(t)) }))
	defer srv.Close()
	s, _ := NewStore("https://worker.invalid/dashboard", "test", filepath.Join(t.TempDir(), "cache.json"))
	s.URL, s.Client = srv.URL, srv.Client()
	snapshot, _ := Parse(fixture(t), fixtureTime)
	if err := saveJSON(s.Cache, snapshot); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	// Advance displayed time over 24 hours; CLOCK rendering and tabs only read cache.
	for minute := 0; minute <= 1440; minute++ {
		_ = s.View(fixtureTime.Add(time.Duration(minute) * time.Minute))
	}
	cancel()
	<-done
	if gets.Load() != 0 || s.View(time.Now()).Snapshot == nil {
		t.Fatal("idle/startup contacted cloud or lost cache")
	}
}

func TestManualRefreshSingleFlightAndOfflineLKG(t *testing.T) {
	var gets, measures atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gets.Add(1); w.Write(fixture(t)) }))
	defer srv.Close()
	s, _ := NewStore("https://worker.invalid/dashboard", "test", filepath.Join(t.TempDir(), "cache.json"))
	s.URL, s.Client = srv.URL, srv.Client()
	release := make(chan struct{})
	measure := func(ctx context.Context) (Metrics, error) { measures.Add(1); <-release; return Metrics{}, nil }
	if !s.Refresh(context.Background(), measure) {
		t.Fatal("first tap refused")
	}
	for i := 0; i < 100; i++ {
		if s.Refresh(context.Background(), measure) {
			t.Fatal("duplicate accepted")
		}
	}
	close(release)
	until := time.Now().Add(2 * time.Second)
	for s.RefreshStatus() == "更新中" && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if gets.Load() != 1 || measures.Load() != 1 || s.RefreshStatus() != "PC未更新 / LKG" {
		t.Fatal("refresh unbounded or incorrect result")
	}
	if s.Refresh(context.Background(), measure) {
		t.Fatal("cooldown bypassed")
	}
	if s.View(time.Now()).Snapshot.AI == nil || !*s.View(time.Now()).Snapshot.AI.Codex.Stale {
		t.Fatal("offline lost or live-labelled LKG")
	}
}

func TestLANMetricsFreshnessAndIndependentLKG(t *testing.T) {
	snapshot, _ := Parse(hostPayload(t, hostFixture(100)), fixtureTime)
	s := &Store{Cache: filepath.Join(t.TempDir(), "cache.json"), snapshot: snapshot}
	started := fixtureTime.Add(time.Second)
	now := started.Add(time.Second)
	ai := *snapshot.AI
	ai.Updated = now
	host := hostFixture(200)
	host.Measured = now
	a, b, _, e := s.applyMetrics(Metrics{AI: &ai, Host: host}, started, now)
	if !a || !b || e != nil || !s.snapshot.Generated.Equal(fixtureTime) {
		t.Fatal("fresh lanes or canonical generation corrupted")
	}
	s.failed = true
	if aiStatus(s.snapshot.AI, now, s.View(now).Status) != "LIVE" {
		t.Fatal("Vault failure hid successful LAN AI")
	}
	if label, _ := HostDisplay(s.snapshot.Host, now, false); label != "CODEX TEMP" {
		t.Fatal("Vault failure hid successful LAN HOST")
	}
	old := hostFixture(0)
	a, b, _, e = s.applyMetrics(Metrics{Host: old}, now, now.Add(time.Second))
	if a || b || e != nil || *s.snapshot.Host.Temp.Bytes != 200 || !*s.snapshot.Host.Stale || !*s.snapshot.AI.Codex.Stale {
		t.Fatal("old/omitted metrics regressed LKG")
	}
	restored := &Store{Cache: s.Cache}
	restored.Restore(now.Add(time.Second))
	if restored.snapshot == nil || *restored.snapshot.Host.Temp.Bytes != 200 {
		t.Fatal("LAN cache not durable")
	}
}

func TestPTTCorrelatedMeasurementAndVoiceHeartbeat(t *testing.T) {
	p := &PTT{Token: strings.Repeat("a", 32), AllowedIP: "127.0.0.1"}
	c, srv := dialPTT(t, p)
	defer srv.Close()
	defer c.Close()
	readOp(t, c, "ready")
	result := make(chan Metrics, 1)
	go func() {
		m, e := p.RequestMetrics(context.Background())
		if e != nil {
			t.Error(e)
		}
		result <- m
	}()
	var request command
	if e := c.ReadJSON(&request); e != nil || request.Op != "dashboard_refresh" || !sessionID.MatchString(request.ID) {
		t.Fatal("invalid LAN request")
	}
	if e := c.WriteJSON(map[string]any{"op": "dashboard_result", "id": strings.Repeat("b", 32), "host": hostFixture(0)}); e != nil {
		t.Fatal(e)
	}
	c.WriteJSON(command{"ping", ""})
	readOp(t, c, "pong")
	select {
	case <-result:
		t.Fatal("uncorrelated result accepted")
	default:
	}
	c.WriteJSON(map[string]any{"op": "dashboard_result", "id": request.ID, "host": hostFixture(123)})
	select {
	case m := <-result:
		if m.Host == nil || *m.Host.Temp.Bytes != 123 {
			t.Fatal("result lost")
		}
	case <-time.After(time.Second):
		t.Fatal("result timeout")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, e := p.RequestMetrics(ctx); e == nil {
		t.Fatal("timeout accepted")
	}
	// Existing microphone control message size and shape remain unchanged.
	raw, _ := json.Marshal(command{"ping", ""})
	if len(raw) > 512 {
		t.Fatal("voice contract changed")
	}
}
