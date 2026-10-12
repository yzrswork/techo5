package yzrs

import (
	"context"
	"errors"
	"time"
)

type Metrics struct {
	AI   *AI          `json:"ai,omitempty"`
	Host *HostMetrics `json:"host,omitempty"`
}

// Refresh is single-flight with a cooldown after completion. Tab changes do not call it.
// One Worker GET and one authenticated LAN measurement; no retries or cloud polling.
func (s *Store) Refresh(ctx context.Context, measure func(context.Context) (Metrics, error)) bool {
	s.mu.Lock()
	if s.refreshBusy || time.Now().Before(s.refreshUntil) {
		s.mu.Unlock()
		return false
	}
	s.refreshBusy = true
	s.refreshStatus = "更新中"
	s.mu.Unlock()
	go func() {
		inner, cancel := context.WithTimeout(ctx, 65*time.Second)
		defer cancel()
		started := time.Now()
		cloudErr := s.Fetch(inner, started)
		var m Metrics
		pcErr := errors.New("PC unavailable")
		if measure != nil {
			m, pcErr = measure(inner)
		}
		aiOK, hostOK, cacheErr := s.applyMetrics(m, started, time.Now())
		status := "更新済"
		if pcErr != nil || !aiOK || !hostOK {
			status = "PC未更新 / LKG"
		}
		if cloudErr != nil {
			status = "VAULT未更新 / LKG"
		}
		if cacheErr != nil {
			status = "保存失敗 / LKG"
		}
		s.mu.Lock()
		s.refreshStatus = status
		s.refreshBusy = false
		s.refreshUntil = time.Now().Add(5 * time.Second)
		s.mu.Unlock()
	}()
	return true
}

func (s *Store) RefreshStatus() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.refreshStatus
}

// Local measurements retain Vault's generation timestamp and independently monotonic lanes.
// Omitted, invalid, unhealthy or old results stale the previous validated measurement.
func (s *Store) applyMetrics(m Metrics, started, now time.Time) (bool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshot == nil {
		return false, false, errors.New("no validated snapshot")
	}
	next := *s.snapshot
	aiOK := validAI(m.AI, now) && !m.AI.Updated.Before(started) && !m.AI.Updated.After(now) && m.AI.Live(now) &&
		(next.AI == nil || m.AI.Updated.After(next.AI.Updated))
	hostOK := validHost(m.Host, now) && !m.Host.Measured.Before(started) && m.Host.live(now) &&
		(next.Host == nil || m.Host.Measured.After(next.Host.Measured))
	if aiOK {
		next.AI = m.AI
	} else if next.AI != nil {
		a := *next.AI
		stale := true
		a.Codex.Stale = &stale
		next.AI = &a
	}
	if hostOK {
		next.Host = m.Host
	} else if next.Host != nil {
		h := *next.Host
		stale := true
		h.Stale = &stale
		next.Host = &h
	}
	if err := saveJSON(s.Cache, &next); err != nil {
		return false, false, err
	}
	s.snapshot = &next
	return aiOK, hostOK, nil
}
