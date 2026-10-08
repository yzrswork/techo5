// Package yzrs is the portable YZRS dashboard and PTT protocol, independent of hardware and HA.
package yzrs

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"
	"unicode/utf8"
)

const MaxSnapshot = 128 * 1024

var JST = time.FixedZone("Asia/Tokyo", 9*3600)
var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type Window struct {
	Remaining *float64  `json:"remainingPercent"`
	Reset     time.Time `json:"resetsAt"`
}
type Codex struct {
	Plan    string `json:"plan"`
	Status  string `json:"status"`
	Stale   *bool  `json:"stale"`
	Tokens  *int64 `json:"todayTokens"`
	Session Window `json:"session"`
	Weekly  Window `json:"weekly"`
}
type AI struct {
	Source  string    `json:"source"`
	Updated time.Time `json:"updatedAt"`
	Codex   Codex     `json:"codex"`
}
type Activity struct {
	At    time.Time `json:"occurred_at"`
	Kind  string    `json:"kind"`
	Title string    `json:"title"`
}
type Snapshot struct {
	Version   int       `json:"schema_version"`
	Date      string    `json:"data_date"`
	Generated time.Time `json:"generated_at"`
	Source    struct {
		SHA    string `json:"main_sha"`
		Window struct {
			From *string `json:"from_sha"`
			To   string  `json:"to_sha"`
		} `json:"commit_window"`
	} `json:"source"`
	Today struct {
		Notes    *int64 `json:"notes_total"`
		NewNotes *int64 `json:"new_notes"`
		Commits  *int64 `json:"commits_since_snapshot"`
		Projects *int64 `json:"projects"`
	} `json:"today"`
	Activity []Activity `json:"activity"`
	AI       *AI        `json:"ai,omitempty"`
}

func Parse(raw []byte, now time.Time) (*Snapshot, error) {
	bad := errors.New("invalid dashboard snapshot") // Never include response bodies or credentials.
	if len(raw) > MaxSnapshot || !utf8.Valid(raw) {
		return nil, bad
	}
	var s Snapshot
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil {
		return nil, bad
	}
	aiRaw := root["ai"]

	if activity, ok := root["activity"]; !ok || len(activity) == 0 || activity[0] != '[' {
		return nil, bad
	}
	delete(root, "ai")
	mainRaw, _ := json.Marshal(root)
	if json.Unmarshal(mainRaw, &s) != nil || s.Version != 1 || s.Generated.IsZero() || s.Generated.After(now.Add(5*time.Minute)) || s.Date != s.Generated.In(JST).Format("2006-01-02") || !shaPattern.MatchString(s.Source.SHA) || s.Source.Window.To != s.Source.SHA || len(s.Activity) > 20 {
		return nil, bad
	}
	if len(aiRaw) > 0 {
		var ai AI
		if json.Unmarshal(aiRaw, &ai) == nil {
			s.AI = &ai
		}
	}
	if s.Source.Window.From != nil && !shaPattern.MatchString(*s.Source.Window.From) {
		return nil, bad
	}
	if s.Source.Window.From == nil && s.Today.Commits != nil {
		return nil, bad
	}
	// Required nullable counts must exist; absent fields are not silently treated as null.
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil || json.Unmarshal(root["today"], &fields) != nil {
		return nil, bad
	}
	for _, k := range []string{"notes_total", "new_notes", "commits_since_snapshot", "projects"} {
		if _, ok := fields[k]; !ok {
			return nil, bad
		}
	}
	for _, n := range []*int64{s.Today.Notes, s.Today.NewNotes, s.Today.Commits, s.Today.Projects} {
		if n != nil && (*n < 0 || *n > 9007199254740991) {
			return nil, bad
		}
	}
	for _, a := range s.Activity {
		if a.At.IsZero() || utf8.RuneCountInString(a.Title) > 48 || (a.Kind != "pr_merged" && a.Kind != "work_note_updated" && a.Kind != "curated") {
			return nil, bad
		}
	}
	if !validAI(s.AI, now) {
		s.AI = nil
	} // Optional AI faults must not break TODAY.
	return &s, nil
}
func validAI(a *AI, now time.Time) bool {
	if a == nil || a.Source != "token-monitor" || a.Updated.IsZero() || a.Updated.After(now.Add(5*time.Minute)) || a.Codex.Plan == "" || utf8.RuneCountInString(a.Codex.Plan) > 64 || a.Codex.Status == "" || len(a.Codex.Status) > 32 || a.Codex.Stale == nil || a.Codex.Tokens == nil || *a.Codex.Tokens < 0 || *a.Codex.Tokens > 9007199254740991 {
		return false
	}
	for _, w := range []Window{a.Codex.Session, a.Codex.Weekly} {
		if w.Remaining == nil || *w.Remaining < 0 || *w.Remaining > 100 || w.Reset.IsZero() {
			return false
		}
	}
	return true
}
func (a *AI) Live(now time.Time) bool {
	return validAI(a, now) && !*a.Codex.Stale && a.Codex.Status == "ok" && now.Sub(a.Updated) <= 15*time.Minute
}
