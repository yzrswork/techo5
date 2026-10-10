// yzrs-preview draws the native dashboard on a workstation, with no device access.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/yzrs"
)

func main() {
	out := flag.String("out", ".", "output directory")
	fixture := flag.String("snapshot", "", "representative or real Worker response")
	cases := flag.Bool("cases", false, "also render long/empty/offline/unavailable/voice fixture cases")
	connected := flag.Bool("voice-connected", false, "simulate a connected client for preview only")
	hostCases := flag.Bool("host-cases", false, "render fresh, zero, stale and unavailable host fixtures")
	previewNow := flag.String("now", "", "explicit RFC3339 preview clock (for independently updated host data)")
	flag.Parse()
	now := time.Now()
	view := yzrs.View{Status: "NO DATA"}
	if *fixture != "" {
		raw, e := os.ReadFile(*fixture)
		check(e)
		var meta struct {
			Generated time.Time `json:"generated_at"`
		}
		check(json.Unmarshal(raw, &meta))
		now = meta.Generated
		if *previewNow != "" {
			now, e = time.Parse(time.RFC3339Nano, *previewNow)
			check(e)
		}
		s, e := yzrs.Parse(raw, now)
		check(e)
		view = yzrs.View{Snapshot: s, Status: "LIVE"}
	}
	r, e := yzrs.NewRenderer()
	check(e)
	defer r.Close()
	check(os.MkdirAll(*out, 0755))
	render := func(name string, frame yzrs.Frame) {
		img := image.NewRGBA(image.Rect(0, 0, 960, 480))
		r.Draw(img, frame)
		f, e := os.Create(filepath.Join(*out, name+".png"))
		check(e)
		check(png.Encode(f, img))
		check(f.Close())
	}
	base := yzrs.Frame{Now: now, Data: view, Voice: yzrs.PTTView{Connected: *connected, Phase: "IDLE"}, Deck: "UNAVAILABLE"}
	for i, name := range yzrs.Modes {
		frame := base
		frame.Mode = i
		render(name, frame)
	}
	if *hostCases {
		if view.Snapshot == nil {
			check(fmt.Errorf("host cases requires a validated snapshot"))
		}
		for _, c := range []struct {
			name   string
			bytes  int64
			age    time.Duration
			absent bool
		}{
			{"HOST-fresh-fixture", 1524713390, 0, false},
			{"HOST-zero-fixture", 0, 0, false},
			{"HOST-stale-fixture", 1524713390, 16 * time.Minute, false},
			{"HOST-unavailable-fixture", 0, 0, true},
		} {
			s := *view.Snapshot
			stale := false
			s.Host = &yzrs.HostMetrics{Version: 1, Source: "windows-codex-temp", Measured: now.Add(-c.age),
				Temp: yzrs.CodexTemp{Bytes: &c.bytes, Status: "ok"}, Stale: &stale}
			if c.absent {
				s.Host = nil
			}
			frame := base
			frame.Data.Snapshot = &s
			render(c.name, frame)
		}
	}
	if *cases {
		if view.Snapshot == nil {
			check(fmt.Errorf("cases requires a validated snapshot fixture"))
		}
		frame := base
		frame.Mode = 1
		long := *view.Snapshot
		long.Activity = append([]yzrs.Activity(nil), long.Activity...)
		if len(long.Activity) == 0 {
			long.Activity = []yzrs.Activity{{At: now, Kind: "curated", Title: "日本語の活動"}}
		}
		for i := range long.Activity {
			long.Activity[i].Title = strings.Repeat("日本語の長い活動内容", 4)
		}
		long.Activity = append(long.Activity, long.Activity[0], long.Activity[0])
		frame.Data.Snapshot = &long
		render("TODAY-long-ja", frame)
		empty := *view.Snapshot
		empty.Activity = []yzrs.Activity{}
		frame.Data.Snapshot = &empty
		render("TODAY-empty", frame)
		missing := empty
		missing.AI = nil
		missing.Today.Notes, missing.Today.NewNotes, missing.Today.Commits = nil, nil, nil
		frame.Mode = 2
		frame.Data.Snapshot = &missing
		render("AI-unavailable", frame)
		frame = base
		frame.Data.Status = "OFFLINE / LKG"
		frame.Mode = 1
		render("TODAY-offline-lkg", frame)
		frame.Mode = 2
		render("AI-offline-lkg", frame)
		frame.Now = now.Add(3 * time.Hour)
		frame.Data.Status = "STALE"
		render("AI-stale", frame)
		frame = base
		frame.Mode = 1
		frame.Data = yzrs.View{Status: "NO DATA"}
		render("TODAY-no-data", frame)
		frame = base
		frame.Mode = 3
		frame.Voice = yzrs.PTTView{Connected: true, Phase: "LISTENING"}
		frame.Deck = "PAIRED PC READY"
		render("VOICE-listening-fixture", frame)
		frame.Voice.Phase = "IDLE"
		render("VOICE-idle-fixture", frame)
		frame.Voice.Phase = "TRANSCRIBING"
		render("VOICE-transcribing-fixture", frame)
	}
	fmt.Println("960x480 native frames rendered")
}
func check(e error) {
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
