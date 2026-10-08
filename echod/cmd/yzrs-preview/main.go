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
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/yzrs"
)

func main() {
	out := flag.String("out", ".", "output directory")
	fixture := flag.String("snapshot", "", "representative or real Worker response")
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
		s, e := yzrs.Parse(raw, now)
		check(e)
		view = yzrs.View{Snapshot: s, Status: "LIVE"}
	}
	r, e := yzrs.NewRenderer()
	check(e)
	defer r.Close()
	check(os.MkdirAll(*out, 0755))
	for i, name := range yzrs.Modes {
		img := image.NewRGBA(image.Rect(0, 0, 960, 480))
		r.Draw(img, yzrs.Frame{Now: now, Mode: i, Data: view, Voice: yzrs.PTTView{Connected: true, Phase: "IDLE"}, Deck: "PAIRED PC READY"})
		f, e := os.Create(filepath.Join(*out, name+".png"))
		check(e)
		check(png.Encode(f, img))
		check(f.Close())
	}
	fmt.Println("960x480 native frames rendered")
}
func check(e error) {
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
