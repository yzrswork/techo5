// A workstation-only synthetic PCM server for P2A; it never opens a microphone.
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/yzrs"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:17329", "loopback test address")
	cert := flag.String("cert", "", "test TLS certificate")
	key := flag.String("key", "", "test TLS private key")
	pcmPath := flag.String("pcm", "", "optional existing private 16k mono PCM16LE fixture (never committed)")
	flag.Parse()
	var sample []byte
	if *pcmPath != "" {
		var e error
		sample, e = os.ReadFile(*pcmPath)
		if e != nil || len(sample) > 1920000 || len(sample)%640 != 0 {
			panic("invalid bounded PCM fixture")
		}
	}
	p := &yzrs.PTT{Token: "fixture-token-loopback-only-000000", AllowedIP: "127.0.0.1", Listen: func() (<-chan []int16, func()) {
		ctx, cancel := context.WithCancel(context.Background())
		frames := make(chan []int16, 1)
		go func() {
			defer close(frames)
			tick := time.NewTicker(20 * time.Millisecond)
			defer tick.Stop()
			offset := 0
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					frame := make([]int16, 320)
					if len(sample) > 0 {
						if offset+640 > len(sample) {
							continue
						}
						for i := range frame {
							frame[i] = int16(binary.LittleEndian.Uint16(sample[offset+i*2:]))
						}
						offset += 640
					}
					select {
					case frames <- frame:
					default:
					}
				}
			}
		}()
		return frames, cancel
	}}
	if *addr != "127.0.0.1:17329" {
		panic("fixture must stay on loopback")
	}
	srv := &http.Server{Addr: *addr, Handler: p.Handler(context.Background()), ReadHeaderTimeout: 3 * time.Second}
	fmt.Println("P2A fixture: loopback only, synthetic silent PCM")
	if e := srv.ListenAndServeTLS(*cert, *key); e != nil {
		panic(e)
	}
}
