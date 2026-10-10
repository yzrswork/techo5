package yzrs

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/image/font"
)

func hostFixture(n int64) *HostMetrics {
	stale := false
	return &HostMetrics{Version: 1, Source: "windows-codex-temp", Measured: fixtureTime, Temp: CodexTemp{Bytes: &n, Status: "ok"}, Stale: &stale}
}
func hostPayload(t *testing.T, host any) []byte {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(fixture(t), &root); err != nil {
		t.Fatal(err)
	}
	root["host"] = host
	raw, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestHostOptionalValidation(t *testing.T) {
	h := hostFixture(6912)
	s, err := Parse(hostPayload(t, h), fixtureTime)
	if err != nil || s.Host == nil || !s.Host.live(fixtureTime) {
		t.Fatal("valid metric lost")
	}
	for _, raw := range []string{
		`null`, `{}`, `{"schemaVersion":1,"source":"windows-codex-temp","measuredAt":"2026-10-08T12:34:00+09:00","codexTemp":{"status":"ok"},"stale":false}`,
	} {
		var v any
		json.Unmarshal([]byte(raw), &v)
		s, err = Parse(hostPayload(t, v), fixtureTime)
		if err != nil || s.Host != nil || s.AI == nil || *s.Today.Notes != 2101 {
			t.Fatal("optional invalid host broke TODAY/AI")
		}
	}
	for _, n := range []int64{-1, MaxHostBytes + 1} {
		s, err = Parse(hostPayload(t, hostFixture(n)), fixtureTime)
		if err != nil || s.Host != nil {
			t.Fatal("invalid bytes accepted")
		}
	}
	for _, change := range []func(*HostMetrics){
		func(h *HostMetrics) { h.Measured = fixtureTime.Add(time.Nanosecond) },
		func(h *HostMetrics) { h.Temp.Bytes = nil }, func(h *HostMetrics) { h.Temp.Status = "invalid" },
		func(h *HostMetrics) { h.Source = "hostname" }, func(h *HostMetrics) { h.Version = 2 },
		func(h *HostMetrics) { h.Stale = nil }, func(h *HostMetrics) { h.Temp.Status = "partial" },
	} {
		h = hostFixture(0)
		change(h)
		s, err = Parse(hostPayload(t, h), fixtureTime)
		if err != nil || s.Host != nil {
			t.Fatal("invalid host accepted")
		}
	}
	h = hostFixture(0)
	h.Temp.Status = "no-matches"
	s, err = Parse(hostPayload(t, h), fixtureTime)
	if err != nil || s.Host == nil || *s.Host.Temp.Bytes != 0 {
		t.Fatal("true zero unavailable")
	}
	h.Temp.Bytes = nil
	h.Temp.Status = "access-denied"
	*h.Stale = true
	s, err = Parse(hostPayload(t, h), fixtureTime)
	if err != nil || s.Host == nil || s.Host.usable() {
		t.Fatal("failure fabricated zero")
	}
	// Explicit null and absent are distinct on the wire; both are unusable for ok.
	var root map[string]any
	json.Unmarshal(hostPayload(t, hostFixture(1)), &root)
	for _, bad := range []any{1.5, "0", true, nil} {
		rawHost := root["host"].(map[string]any)
		rawHost["codexTemp"].(map[string]any)["bytes"] = bad
		raw, _ := json.Marshal(root)
		s, err = Parse(raw, fixtureTime)
		if err != nil || s.Host != nil {
			t.Fatal("wrong byte type accepted")
		}
	}
}

func TestHostDisplayAndPreservation(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, tc := range []struct {
		h            *HostMetrics
		age          time.Duration
		offline      bool
		label, value string
	}{
		{nil, 0, false, "CODEX TEMP", "UNAVAILABLE"},
		{hostFixture(0), 0, false, "CODEX TEMP", "0 MB"},
		{hostFixture(346911), 0, false, "CODEX TEMP", "<1 MB"},
		{hostFixture(386 << 20), 0, false, "CODEX TEMP", "386 MB"},
		{hostFixture(1524713390), 0, false, "CODEX TEMP", "1.42 GB"},
		{hostFixture(MaxHostBytes), 0, false, "CODEX TEMP", "16384.00 GB"},
		{hostFixture(1524713390), 16 * time.Minute, false, "CODEX TEMP STALE", "1.42 GB"},
		{hostFixture(0), 0, true, "CODEX TEMP STALE", "0 MB"},
	} {
		label, value := HostDisplay(tc.h, fixtureTime.Add(tc.age), tc.offline)
		if label != tc.label || value != tc.value {
			t.Fatalf("got %s / %s", label, value)
		}
		for _, line := range []struct {
			text  string
			small bool
		}{{label, true}, {value, false}} {
			base := r.normal
			if line.small {
				base = r.small
			}
			if w := font.MeasureString(r.faceFor(line.text, base), line.text).Ceil(); w > hudWidth {
				t.Fatalf("HUD clips %q width=%d", line.text, w)
			}
		}
	}
	s, _ := Parse(fixture(t), fixtureTime)
	frame := Frame{Now: fixtureTime, Data: View{Snapshot: s, Status: "LIVE"}}
	for mode := range Modes {
		frame.Mode = mode
		s.Host = nil
		before := image.NewRGBA(image.Rect(0, 0, 960, 480))
		r.Draw(before, frame)
		s.Host = hostFixture(1524713390)
		after := image.NewRGBA(before.Bounds())
		r.Draw(after, frame)
		changed := false
		for y := 0; y < 480; y++ {
			for x := 0; x < 960; x++ {
				if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
					changed = true
					if x < hudLeft || x >= hudLeft+hudWidth || y < 258 || y > 309 {
						t.Fatalf("unrelated pixel changed: %d,%d mode=%d", x, y, mode)
					}
				}
			}
		}
		if !changed {
			t.Fatal("numeric host metric not rendered")
		}
	}
}

func TestHostStoreIndependentLKG(t *testing.T) {
	raw := hostPayload(t, hostFixture(6912))
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(raw) }))
	defer srv.Close()
	st, _ := NewStore("https://worker.invalid/dashboard", "test-token", filepath.Join(t.TempDir(), "lkg.json"))
	st.URL = srv.URL
	st.Client = srv.Client()
	if err := st.Fetch(context.Background(), fixtureTime); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(st.Cache)
	for _, h := range []*HostMetrics{nil, hostFixture(1), hostFixture(0)} {
		if h != nil {
			h.Measured = fixtureTime.Add(-time.Minute)
		}
		raw = hostPayload(t, h)
		if err := st.Fetch(context.Background(), fixtureTime); err != nil {
			t.Fatal(err)
		}
		v := st.View(fixtureTime).Snapshot
		if v.Host == nil || *v.Host.Temp.Bytes != 6912 || !*v.Host.Stale || v.AI == nil || *v.Today.Notes != 2101 {
			t.Fatal("LKG/regression/isolation failed")
		}
	}
	h := hostFixture(12345)
	h.Measured = fixtureTime.Add(time.Minute)
	raw = hostPayload(t, h)
	if err := st.Fetch(context.Background(), h.Measured); err != nil {
		t.Fatal(err)
	}
	if v := st.View(h.Measured).Snapshot.Host; *v.Temp.Bytes != 12345 || *v.Stale {
		t.Fatal("host recovery failed")
	}
	raw = hostPayload(t, h)
	if err := st.Fetch(context.Background(), h.Measured); err != nil {
		t.Fatal(err)
	}
	if *st.View(h.Measured).Snapshot.Host.Stale {
		t.Fatal("identical fresh repeat marked stale")
	}
	saved, _ := os.ReadFile(st.Cache)
	if bytes.Equal(saved, first) {
		t.Fatal("host cache not updated")
	}
	restored, _ := NewStore("https://worker.invalid/dashboard", "test-token", st.Cache)
	restored.Restore(h.Measured)
	label, _ := HostDisplay(restored.View(h.Measured).Snapshot.Host, h.Measured, true)
	if label != "CODEX TEMP STALE" {
		t.Fatal("offline restart falsely fresh")
	}
}
