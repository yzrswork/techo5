//go:build !dot && !spot

// Package yzrs wires the narrow YZRS client to TECHO5's existing microphone and Deck.
package yzrs

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/component"
	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/feature/deck"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/mic"
	"github.com/HuskerMinion/techo5/echod/internal/layout"
	"github.com/HuskerMinion/techo5/echod/internal/service"
	core "github.com/HuskerMinion/techo5/echod/internal/yzrs"
)

type Settings struct {
	Enabled      bool   `json:"enabled"`
	DashboardURL string `json:"dashboard_url"`
	ReadToken    string `json:"dashboard_read_token"`
	PTTAddr      string `json:"ptt_addr"`
	PCIP         string `json:"pc_ip"`
	PTTToken     string `json:"ptt_token"`
	TLSCert      string `json:"tls_cert"`
	TLSKey       string `json:"tls_key"`
	VoicePage    int    `json:"voice_deck_page"`
	VoiceButton  int    `json:"voice_deck_button"`
}
type Feature struct {
	mu           sync.RWMutex
	enabled      bool
	cfg          Settings
	store        *core.Store
	ptt          *core.PTT
	mode         int
	actionBusy   bool
	actionStatus string
}

var shared = &Feature{}

func Get() *Feature { return shared }
func init() {
	component.Register(component.Device, Get(), component.Order(39), component.Supervise(service.Restart(time.Second, 30*time.Second)))
}
func (f *Feature) Name() string  { return "YZRS dashboard / PTT" }
func (f *Feature) Enabled() bool { f.mu.RLock(); defer f.mu.RUnlock(); return f.enabled }
func (f *Feature) Run(ctx context.Context) error {
	// Config is deliberately separate from HA's key and upstream's persisted config.
	file, e := os.Open(layout.StateDir + "/yzrs.json")
	if os.IsNotExist(e) {
		<-ctx.Done()
		return nil
	}
	if e != nil {
		return errors.New("YZRS config unavailable")
	}
	var cfg Settings
	d := json.NewDecoder(io.LimitReader(file, 8193))
	d.DisallowUnknownFields()
	e = d.Decode(&cfg)
	file.Close()
	if e != nil {
		return errors.New("invalid YZRS config")
	}
	if !cfg.Enabled {
		<-ctx.Done()
		return nil
	}
	store, e := core.NewStore(cfg.DashboardURL, cfg.ReadToken, layout.StateDir+"/yzrs-dashboard.json")
	if e != nil {
		return e
	}
	var ptt *core.PTT
	var server *http.Server
	inner, cancel := context.WithCancel(ctx)
	defer cancel()
	if cfg.PTTAddr != "" {
		host, port, e := net.SplitHostPort(cfg.PTTAddr)
		ip := net.ParseIP(host)
		if e != nil || port != "17327" || ip == nil || ip.To4() == nil || !ip.IsPrivate() || net.ParseIP(cfg.PCIP) == nil || net.ParseIP(cfg.PCIP).To4() == nil || !net.ParseIP(cfg.PCIP).IsPrivate() || len(cfg.PTTToken) < 32 {
			return errors.New("invalid trusted LAN PTT configuration")
		}
		cert, e := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if e != nil {
			return errors.New("PTT TLS identity unavailable")
		}
		cleanup, e := pttFirewall(cfg.PCIP)
		if e != nil {
			return e
		}
		defer cleanup()
		ptt = &core.PTT{Token: cfg.PTTToken, AllowedIP: cfg.PCIP, Listen: func() (<-chan []int16, func()) { return mic.Get().Listen("yzrs-ptt") }}
		server = &http.Server{Handler: ptt.Handler(inner), ReadHeaderTimeout: 3 * time.Second, IdleTimeout: 10 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}}
	}
	f.mu.Lock()
	f.cfg = cfg
	f.store = store
	f.ptt = ptt
	f.enabled = true
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.enabled = false; f.mu.Unlock() }()
	done := make(chan struct{})
	go func() { defer close(done); store.Run(inner) }()
	defer func() { cancel(); <-done }()
	if server == nil {
		<-ctx.Done()
		return nil
	}
	// Wi-Fi may not own its reserved address yet. Dashboard remains available while PTT waits.
	var listener net.Listener
	for {
		listener, e = net.Listen("tcp", cfg.PTTAddr)
		if e == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
	}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(tls.NewListener(listener, server.TLSConfig)) }()
	defer server.Close()
	select {
	case <-ctx.Done():
		return nil
	case <-finished:
		return errors.New("PTT server stopped")
	}
}
func (f *Feature) Frame(now time.Time) core.Frame {
	f.mu.RLock()
	st, p, mode, status, cfg := f.store, f.ptt, f.mode, f.actionStatus, f.cfg
	f.mu.RUnlock()
	v := core.Frame{Now: now, Mode: mode, Data: core.View{Status: "NO DATA"}, Deck: status}
	if st != nil {
		v.Data = st.View(now)
	}
	if p != nil {
		v.Voice = p.View()
	}
	if v.Deck == "" {
		b := config.Get().Deck.Button(cfg.VoicePage, cfg.VoiceButton)
		pc, ok := deck.Get().Computers()[b.Computer]
		v.Deck = "UNAVAILABLE"
		if b.Action == config.DeckPCRun && ok && pc.Hello != nil && pc.Err == "" {
			v.Deck = "PAIRED PC READY"
		}
	}
	return v
}
func (f *Feature) Select(mode int) {
	if mode < 0 || mode > 3 {
		return
	}
	f.mu.Lock()
	f.mode = mode
	f.mu.Unlock()
	if mode == 3 {
		f.EnsureVoice()
	}
}
func (f *Feature) EnsureVoice() {
	f.mu.Lock()
	if f.actionBusy {
		f.mu.Unlock()
		return
	}
	cfg := f.cfg
	f.actionBusy = true
	f.actionStatus = "STARTING"
	f.mu.Unlock()
	go func() {
		b := config.Get().Deck.Button(cfg.VoicePage, cfg.VoiceButton)
		status := "UNAVAILABLE"
		// Only a named, configured script on an already paired computer. No shell commands here.
		if cfg.VoicePage >= 0 && cfg.VoiceButton >= 0 && b.Action == config.DeckPCRun && b.Value != "" {
			if deck.Get().Press(cfg.VoicePage, cfg.VoiceButton) == nil {
				status = "START REQUEST SENT"
			}
		}
		f.mu.Lock()
		f.actionStatus = status
		f.actionBusy = false
		f.mu.Unlock()
	}()
}
