package yzrs

import (
	"context"
	"encoding/json"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var fixtureTime = time.Date(2026, 10, 8, 12, 34, 0, 0, JST)

func fixture(t *testing.T) []byte {
	t.Helper()
	raw, e := os.ReadFile("testdata/dashboard.json")
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func TestContractAndOptionalAI(t *testing.T) {
	raw := fixture(t)
	s, e := Parse(raw, fixtureTime)
	if e != nil {
		t.Fatal(e)
	}
	if s.AI == nil || !s.AI.Live(fixtureTime) || *s.Today.Notes != 2101 {
		t.Fatal("valid Worker response rejected")
	}
	for _, change := range []struct{ old, new string }{{`"schema_version":1`, `"schema_version":2`}, {`"notes_total":2101`, `"notes_total":-1`}, {`"generated_at":"2026-10-08T12:34:00+09:00"`, `"generated_at":"2026-10-09T12:34:00+09:00"`}} {
		if _, e = Parse([]byte(strings.Replace(string(raw), change.old, change.new, 1)), fixtureTime); e == nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
	for _, bad := range []string{`"todayTokens":"wrong"`, `"todayTokens":-1`, `"todayTokens":1.5`} {
		s, e = Parse([]byte(strings.Replace(string(raw), `"todayTokens":123456`, bad, 1)), fixtureTime)
		if e != nil || s.AI != nil {
			t.Fatal("optional AI fault broke TODAY")
		}
	}
	if s, e = Parse(raw, fixtureTime.Add(16*time.Minute)); e != nil || s.AI.Live(fixtureTime.Add(16*time.Minute)) {
		t.Fatal("AI age not applied")
	}
}
func TestStoreLKGAndRecovery(t *testing.T) {
	raw := fixture(t)
	var state atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-read-token" {
			t.Error("read auth missing")
		}
		switch state.Load() {
		case 1:
			http.Error(w, "failed", 503)
		case 2:
			w.Write([]byte(`{"schema_version":2}`))
		case 3:
			w.Write([]byte(strings.Repeat("x", MaxSnapshot+1)))
		case 4:
			http.Redirect(w, r, "https://other.invalid/dashboard", 302)
		default:
			w.Write(raw)
		}
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "lkg.json")
	st, _ := NewStore("https://worker.invalid/dashboard", "test-read-token", path)
	st.URL = srv.URL
	st.Client = srv.Client()
	st.Client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if e := st.Fetch(context.Background(), fixtureTime); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(path)
	for i := int32(1); i <= 4; i++ {
		state.Store(i)
		if st.Fetch(context.Background(), fixtureTime) == nil {
			t.Fatal("failed response accepted")
		}
		after, _ := os.ReadFile(path)
		if string(before) != string(after) || st.View(fixtureTime).Status != "OFFLINE / LKG" {
			t.Fatal("LKG replaced")
		}
	}
	restored, _ := NewStore("https://worker.invalid/dashboard", "test-read-token", path)
	restored.Restore(fixtureTime)
	if restored.View(fixtureTime).Snapshot == nil {
		t.Fatal("restart lost LKG")
	}
	state.Store(0)
	if st.Fetch(context.Background(), fixtureTime) != nil || st.View(fixtureTime).Status != "LIVE" {
		t.Fatal("network recovery failed")
	}
	if st.View(fixtureTime.Add(3*time.Hour)).Status != "STALE" {
		t.Fatal("snapshot age missing")
	}
}
func TestNativeJapaneseRenderer(t *testing.T) {
	r, e := NewRenderer()
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	for _, ch := range "日本語ダッシュボード今日活動音声入力接続復帰検証年月曜日同期利用不可。…" {
		if !r.HasGlyph(ch) {
			t.Fatalf("missing glyph %U", ch)
		}
	}
	raw := fixture(t)
	s, _ := Parse(raw, fixtureTime)
	for i := range Modes {
		dst := image.NewRGBA(image.Rect(0, 0, 960, 480))
		r.Draw(dst, Frame{Now: fixtureTime, Mode: i, Data: View{Snapshot: s, Status: "LIVE"}})
		if dst.RGBAAt(0, 0) != r.background.RGBAAt(0, 0) {
			t.Fatal("reference background was not drawn")
		}
	}
	for i := range Modes {
		m, ok := ModeAt(NavBounds(i).Min.X+10, 440)
		if !ok || m != i {
			t.Fatal("navigation hit mapping")
		}
	}
	if _, ok := ModeAt(940, 390); ok {
		t.Fatal("outside navigation activated")
	}
}

func TestAIOfflineStatus(t *testing.T) {
	s, e := Parse(fixture(t), fixtureTime)
	if e != nil {
		t.Fatal(e)
	}
	if got := aiStatus(s.AI, fixtureTime, "OFFLINE / LKG"); got != "OFFLINE / LKG" {
		t.Fatalf("cached AI falsely shown live: %s", got)
	}
	if aiStatus(s.AI, fixtureTime, "LIVE") != "LIVE" || aiStatus(s.AI, fixtureTime.Add(time.Hour), "STALE") != "STALE" {
		t.Fatal("AI freshness lost")
	}
}
func dialPTT(t *testing.T, p *PTT) (*websocket.Conn, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(p.Handler(context.Background()))
	c, _, e := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ptt", http.Header{"Authorization": []string{"Bearer " + p.Token}})
	if e != nil {
		srv.Close()
		t.Fatal(e)
	}
	return c, srv
}
func readOp(t *testing.T, c *websocket.Conn, want string) {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg command
	if e := c.ReadJSON(&msg); e != nil {
		t.Fatal(e)
	}
	if msg.Op != want {
		t.Fatalf("op=%s want=%s", msg.Op, want)
	}
}
func TestPTTLifecycleDisconnectAndLease(t *testing.T) {
	var released atomic.Int32
	frames := make(chan []int16, 1)
	p := &PTT{Token: strings.Repeat("a", 32), AllowedIP: "127.0.0.1", Lease: 200 * time.Millisecond, Listen: func() (<-chan []int16, func()) { return frames, func() { released.Add(1) } }}
	c, srv := dialPTT(t, p)
	defer srv.Close()
	readOp(t, c, "ready")
	id := strings.Repeat("b", 32)
	c.WriteJSON(command{"start", id})
	readOp(t, c, "started")
	frames <- make([]int16, 320)
	kind, pcm, e := c.ReadMessage()
	if e != nil || kind != websocket.BinaryMessage || len(pcm) != 640 {
		t.Fatal("native frame mismatch")
	}
	c.WriteJSON(command{"stop", id})
	readOp(t, c, "stopped")
	if p.View().Phase != "TRANSCRIBING" || released.Load() != 1 {
		t.Fatal("stop did not release microphone")
	}
	c.WriteJSON(command{"done", id})
	readOp(t, c, "idle")
	c.WriteJSON(command{"start", id})
	readOp(t, c, "rejected")
	c.WriteJSON(command{"start", strings.Repeat("c", 32)})
	readOp(t, c, "started")
	c.Close()
	until := time.Now().Add(time.Second)
	for released.Load() != 2 && time.Now().Before(until) {
		time.Sleep(5 * time.Millisecond)
	}
	if released.Load() != 2 {
		t.Fatal("disconnect left mic attached")
	}
	c, _, e = websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ptt", http.Header{"Authorization": []string{"Bearer " + p.Token}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	readOp(t, c, "ready")
	c.WriteJSON(command{"start", strings.Repeat("d", 32)})
	readOp(t, c, "started")
	time.Sleep(300 * time.Millisecond)
	if released.Load() != 3 || p.View().Connected {
		t.Fatal("missing heartbeat did not clean up")
	}
}
func TestPTTAuthAndTimeBound(t *testing.T) {
	var release atomic.Int32
	p := &PTT{Token: strings.Repeat("a", 32), AllowedIP: "127.0.0.1", MaxRecording: 40 * time.Millisecond, Listen: func() (<-chan []int16, func()) { return make(chan []int16), func() { release.Add(1) } }}
	srv := httptest.NewServer(p.Handler(context.Background()))
	defer srv.Close()
	res, e := http.Get(srv.URL + "/ptt")
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("unauthenticated mic access")
	}
	c, _, e := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ptt", http.Header{"Authorization": []string{"Bearer " + p.Token}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	readOp(t, c, "ready")
	c.WriteJSON(command{"start", strings.Repeat("b", 32)})
	readOp(t, c, "started")
	readOp(t, c, "aborted")
	if release.Load() != 1 {
		t.Fatal("maximum recording not enforced")
	}
	_, res, e = websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ptt", http.Header{"Authorization": []string{"Bearer " + p.Token}})
	if e == nil || res.StatusCode != 409 {
		t.Fatal("multiple PC clients admitted")
	}
	res.Body.Close()
}
func TestCacheExcludesUnknownFields(t *testing.T) {
	raw := fixture(t)
	var obj map[string]any
	json.Unmarshal(raw, &obj)
	obj["private_unknown"] = "must not persist"
	raw, _ = json.Marshal(obj)
	s, e := Parse(raw, fixtureTime)
	if e != nil {
		t.Fatal(e)
	}
	saved, _ := json.Marshal(s)
	if strings.Contains(string(saved), "private_unknown") {
		t.Fatal("unknown fields retained")
	}
}

func BenchmarkNativeFrame(b *testing.B) {
	r, e := NewRenderer()
	if e != nil {
		b.Fatal(e)
	}
	defer r.Close()
	raw, e := os.ReadFile("testdata/dashboard.json")
	if e != nil {
		b.Fatal(e)
	}
	s, e := Parse(raw, fixtureTime)
	if e != nil {
		b.Fatal(e)
	}
	dst := image.NewRGBA(image.Rect(0, 0, 960, 480))
	f := Frame{Now: fixtureTime, Mode: 1, Data: View{Snapshot: s, Status: "LIVE"}}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r.Draw(dst, f)
	}
}
