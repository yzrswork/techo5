package yzrs

import (
	"encoding/json"
	"fmt"
	"time"
)

const MaxHostBytes int64 = 16 * 1024 * 1024 * 1024 * 1024

type CodexTemp struct {
	Bytes  *int64 `json:"bytes"`
	Status string `json:"status"`
}

// Require the bytes key even when null: a missing measurement is never zero.
func (m *CodexTemp) UnmarshalJSON(raw []byte) error {
	type plain CodexTemp
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields["bytes"] == nil || fields["status"] == nil {
		return fmt.Errorf("invalid host metric")
	}
	var p plain
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("invalid host metric")
	}
	*m = CodexTemp(p)
	return nil
}

type HostMetrics struct {
	Version  int       `json:"schemaVersion"`
	Source   string    `json:"source"`
	Measured time.Time `json:"measuredAt"`
	Temp     CodexTemp `json:"codexTemp"`
	Stale    *bool     `json:"stale"`
}

func (h *HostMetrics) usable() bool {
	return h != nil && h.Temp.Bytes != nil && (h.Temp.Status == "ok" || h.Temp.Status == "no-matches")
}
func validHost(h *HostMetrics, now time.Time) bool {
	if h == nil || h.Version != 1 || h.Source != "windows-codex-temp" || h.Measured.IsZero() || h.Measured.Before(time.Unix(0, 0)) || h.Measured.After(now) || h.Stale == nil {
		return false
	}
	switch h.Temp.Status {
	case "ok", "no-matches":
		return h.Temp.Bytes != nil && *h.Temp.Bytes >= 0 && *h.Temp.Bytes <= MaxHostBytes && (h.Temp.Status != "no-matches" || *h.Temp.Bytes == 0)
	case "partial", "access-denied", "timeout", "error":
		return h.Temp.Bytes == nil && *h.Stale
	}
	return false
}
func (h *HostMetrics) live(now time.Time) bool {
	return validHost(h, now) && h.usable() && !*h.Stale && now.Sub(h.Measured) <= 15*time.Minute
}

// MB/GB labels use binary MiB/GiB conversion. Show sub-MiB nonzero values
// with <1 MB so an actual empty result remains distinguishable.
func HostDisplay(h *HostMetrics, now time.Time, offline bool) (label, value string) {
	label, value = "CODEX TEMP", "UNAVAILABLE"
	if !validHost(h, now) || !h.usable() {
		return
	}
	n := *h.Temp.Bytes
	switch {
	case n == 0:
		value = "0 MB"
	case n < 1<<20:
		value = "<1 MB"
	case n < 1<<30:
		value = fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	default:
		value = fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	}
	if offline || !h.live(now) {
		label += " STALE"
	}
	return
}
