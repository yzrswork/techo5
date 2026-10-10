//go:build !dot && !spot

package yzrs

import (
	"errors"
	"strings"
	"testing"
)

func TestFirewallOwnChainAndExactPeer(t *testing.T) {
	var calls []string
	run := func(args ...string) error {
		s := strings.Join(args, " ")
		calls = append(calls, s)
		if args[0] == "-C" {
			return errors.New("absent")
		}
		return nil
	}
	cleanup, e := configureFirewall(run, "192.168.1.10")
	if e != nil {
		t.Fatal(e)
	}
	cleanup()
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "-s 192.168.1.10 -p tcp --dport 17327 -j ACCEPT") || !strings.Contains(joined, "-D TECHO5-IN -j YZRS-PTT") {
		t.Fatal("peer restriction or cleanup missing")
	}
	for _, call := range calls {
		if call == "-F TECHO5-IN" || call == "-F" {
			t.Fatal("upstream firewall flushed")
		}
	}
}
func TestFirewallFailureStaysClosed(t *testing.T) {
	cleanup, e := configureFirewall(func(args ...string) error {
		if args[0] == "-A" {
			return errors.New("failure")
		}
		return nil
	}, "192.168.1.10")
	if e == nil || cleanup != nil {
		t.Fatal("failed firewall allowed PTT startup")
	}
}
