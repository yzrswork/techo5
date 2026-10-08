package yzrs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type View struct {
	Snapshot *Snapshot
	Status   string
	Checked  time.Time
}
type Store struct {
	URL, Token, Cache string
	Client            *http.Client
	mu                sync.RWMutex
	snapshot          *Snapshot
	checked           time.Time
	failed            bool
}

func NewStore(endpoint, token, cache string) (*Store, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/dashboard" || token == "" {
		return nil, errors.New("invalid read-only dashboard configuration")
	}
	return &Store{URL: endpoint, Token: token, Cache: cache, Client: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (s *Store) Restore(now time.Time) {
	f, e := os.Open(s.Cache)
	if e != nil {
		return
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, MaxSnapshot+1))
	if e != nil {
		return
	}
	parsed, e := Parse(raw, now)
	if e == nil {
		s.mu.Lock()
		s.snapshot = parsed
		s.failed = true
		s.mu.Unlock()
	}
}
func (s *Store) Fetch(ctx context.Context, now time.Time) (err error) {
	defer func() { s.mu.Lock(); s.checked = now; s.failed = err != nil; s.mu.Unlock() }()
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if e != nil {
		return errors.New("dashboard request failed")
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set("Accept", "application/json")
	res, e := s.Client.Do(req)
	if e != nil {
		return errors.New("dashboard unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return errors.New("dashboard HTTP failure")
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, MaxSnapshot+1))
	if e != nil {
		return errors.New("dashboard read failed")
	}
	parsed, e := Parse(raw, now)
	if e != nil {
		return e
	}
	s.mu.RLock()
	previous := s.snapshot
	s.mu.RUnlock()
	if previous != nil && parsed.Generated.Before(previous.Generated) {
		return errors.New("dashboard timestamp regressed")
	}
	// AI updates independently of Vault snapshots. Retain its validated LKG on optional-lane failure.
	if previous != nil && previous.AI != nil && (parsed.AI == nil || parsed.AI.Updated.Before(previous.AI.Updated)) {
		copyAI := *previous.AI
		stale := true
		copyAI.Codex.Stale = &stale
		parsed.AI = &copyAI
	}
	// Persist a validated allowlist, never unknown Worker fields. Atomic rename preserves LKG.
	if e = saveJSON(s.Cache, parsed); e != nil {
		return errors.New("dashboard cache write failed")
	}
	s.mu.Lock()
	s.snapshot = parsed
	s.mu.Unlock()
	return nil
}
func (s *Store) View(now time.Time) View {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := View{Snapshot: s.snapshot, Checked: s.checked, Status: "NO DATA"}
	if s.snapshot != nil {
		v.Status = "LIVE"
		if s.failed {
			v.Status = "OFFLINE / LKG"
		} else if now.Sub(s.snapshot.Generated) > 2*time.Hour || s.snapshot.Date != now.In(JST).Format("2006-01-02") {
			v.Status = "STALE"
		}
	}
	return v
}
func (s *Store) Run(ctx context.Context) {
	s.Restore(time.Now())
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		_ = s.Fetch(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
func saveJSON(path string, value any) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".yzrs-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	raw, e := json.Marshal(value)
	if e == nil {
		_, e = f.Write(raw)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
