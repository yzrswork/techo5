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
	refreshBusy       bool
	refreshUntil      time.Time
	refreshStatus     string
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
		// A restored cache is an LKG, never proof that the PC is currently available.
		stale := true
		if parsed.AI != nil {
			parsed.AI.Codex.Stale = &stale
		}
		if parsed.Host != nil {
			parsed.Host.Stale = &stale
		}
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
	// The Worker is not a source for PC-only lanes; keep the last validated values.
	parsed.Claude, parsed.Times = nil, nil
	if previous != nil {
		parsed.Claude, parsed.Times = previous.Claude, previous.Times
	}
	// Persist a validated allowlist, never unknown Worker fields. Atomic rename preserves LKG.
	// Host observations are independent of both TODAY generation time and AI.
	if previous != nil && previous.Host.usable() &&
		(parsed.Host == nil || !parsed.Host.usable() || !parsed.Host.Measured.After(previous.Host.Measured)) {
		copyHost := *previous.Host
		stale := true
		// An identical fresh observation remains fresh; KV delivery may repeat it.
		if parsed.Host != nil && parsed.Host.usable() && parsed.Host.Measured.Equal(copyHost.Measured) &&
			*parsed.Host.Temp.Bytes == *copyHost.Temp.Bytes && !*parsed.Host.Stale {
			stale = false
		}
		copyHost.Stale = &stale
		parsed.Host = &copyHost
	}
	if e = saveJSON(s.Cache, parsed); e != nil {
		return errors.New("dashboard cache write failed")
	}
	s.mu.Lock()
	s.snapshot = parsed
	s.mu.Unlock()
	return nil
}

// vaultSnapshotDeadline is the first scheduled Vault publication strictly after
// the accepted snapshot, plus the operational grace period. Scheduled times
// are JST (08:00, 12:00, 16:00, 20:00, 23:55), not the Worker GET cadence.
// Manual/push snapshots use the same rule based on their actual generated_at.
func vaultSnapshotDeadline(generated time.Time) time.Time {
	local := generated.In(JST)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, JST)
	for _, offset := range [...]time.Duration{
		8 * time.Hour,
		12 * time.Hour,
		16 * time.Hour,
		20 * time.Hour,
		23*time.Hour + 55*time.Minute,
	} {
		scheduled := midnight.Add(offset)
		if scheduled.After(local) {
			return scheduled.Add(time.Hour)
		}
	}
	// The last daily slot has passed; the next one is 08:00 tomorrow.
	return midnight.AddDate(0, 0, 1).Add(9 * time.Hour)
}

func (s *Store) View(now time.Time) View {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := View{Snapshot: s.snapshot, Checked: s.checked, Status: "NO DATA"}
	if s.snapshot != nil {
		v.Status = "LIVE"
		if s.failed {
			v.Status = "OFFLINE / LKG"
		} else if now.After(vaultSnapshotDeadline(s.snapshot.Generated)) {
			v.Status = "STALE"
		}
	}
	return v
}
func (s *Store) Run(ctx context.Context) {
	s.Restore(time.Now())
	// Startup and idle never contact the Worker. Only an explicit Refresh fetches.
	<-ctx.Done()
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
