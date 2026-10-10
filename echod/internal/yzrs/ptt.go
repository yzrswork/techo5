package yzrs

import (
	"context"
	"crypto/subtle"
	"encoding/binary"
	"net"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Listen func() (<-chan []int16, func())
type PTTView struct {
	Connected bool
	Phase     string
}
type PTT struct {
	Token, AllowedIP    string
	Listen              Listen
	Lease, MaxRecording time.Duration
	mu                  sync.Mutex
	connected           bool
	phase               string
}

func (p *PTT) View() PTTView {
	p.mu.Lock()
	defer p.mu.Unlock()
	phase := p.phase
	if phase == "" {
		phase = "IDLE"
	}
	return PTTView{p.connected, phase}
}
func (p *PTT) setPhase(s string) { p.mu.Lock(); p.phase = s; p.mu.Unlock() }

type command struct {
	Op string `json:"op"`
	ID string `json:"id"`
}

var sessionID = regexp.MustCompile(`^[a-f0-9]{32}$`)

// Handler is served ONLY over TLS. No browser clients; exact PC IP and independent bearer token.
func (p *PTT) Handler(ctx context.Context) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if r.URL.Path != "/ptt" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if len(p.Token) < 32 || p.AllowedIP == "" || host != p.AllowedIP || r.Header.Get("Origin") != "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+p.Token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		p.mu.Lock()
		if p.connected {
			p.mu.Unlock()
			http.Error(w, "busy", 409)
			return
		}
		p.connected = true
		p.phase = "IDLE"
		p.mu.Unlock()
		defer func() { p.mu.Lock(); p.connected = false; p.phase = "IDLE"; p.mu.Unlock() }()
		conn, e := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }, ReadBufferSize: 1024, WriteBufferSize: 1024}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer conn.Close()
		lease := p.Lease
		if lease <= 0 {
			lease = 6 * time.Second
		}
		maximum := p.MaxRecording
		if maximum <= 0 || maximum > 60*time.Second {
			maximum = 60 * time.Second
		}
		inner, cancel := context.WithCancel(ctx)
		defer cancel()
		commands := make(chan command, 8)
		conn.SetReadLimit(512)
		go func() {
			defer cancel()
			for {
				_ = conn.SetReadDeadline(time.Now().Add(lease))
				var c command
				if conn.ReadJSON(&c) != nil {
					return
				}
				select {
				case commands <- c:
				case <-inner.Done():
					return
				default:
					return
				}
			}
		}()
		// Close the socket on daemon shutdown, including a blocked reader.
		go func() { <-inner.Done(); _ = conn.Close() }()
		send := func(op, id string) bool {
			_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			return conn.WriteJSON(command{op, id}) == nil
		}
		if !send("ready", "") {
			return
		}
		var frames <-chan []int16
		var release func()
		stop := func() {
			if release != nil {
				release()
				release = nil
			}
			frames = nil
		}
		defer stop()
		var id, lastID string
		var deadline <-chan time.Time
		var timer *time.Timer
		defer func() {
			if timer != nil {
				timer.Stop()
			}
		}()
		for {
			select {
			case <-inner.Done():
				return
			case <-deadline:
				stop()
				p.setPhase("IDLE")
				deadline = nil
				id = ""
				if !send("aborted", lastID) {
					return
				}
			case c := <-commands:
				switch c.Op {
				case "ping":
					if !send("pong", "") {
						return
					}
				case "start":
					if id != "" || !sessionID.MatchString(c.ID) || c.ID == lastID {
						if !send("rejected", c.ID) {
							return
						}
						continue
					}
					id, lastID = c.ID, c.ID
					frames, release = p.Listen()
					p.setPhase("LISTENING")
					if timer != nil {
						timer.Stop()
					}
					timer = time.NewTimer(maximum)
					deadline = timer.C
					if !send("started", id) {
						return
					}
				case "stop":
					if id == "" || c.ID != id || frames == nil {
						if !send("rejected", c.ID) {
							return
						}
						continue
					}
					stop()
					p.setPhase("TRANSCRIBING")
					timer.Stop()
					timer = time.NewTimer(180 * time.Second)
					deadline = timer.C
					if !send("stopped", id) {
						return
					}
				case "done", "cancel":
					if id == "" || c.ID != id {
						if !send("rejected", c.ID) {
							return
						}
						continue
					}
					stop()
					timer.Stop()
					deadline = nil
					p.setPhase("IDLE")
					id = ""
					if !send("idle", c.ID) {
						return
					}
				default:
					return
				}
			case frame, ok := <-frames:
				if !ok {
					return
				}
				if len(frame) != 320 {
					return
				}
				pcm := make([]byte, 640)
				for i, s := range frame {
					binary.LittleEndian.PutUint16(pcm[i*2:], uint16(s))
				}
				_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
				if conn.WriteMessage(websocket.BinaryMessage, pcm) != nil {
					return
				}
			}
		}
	})
}
