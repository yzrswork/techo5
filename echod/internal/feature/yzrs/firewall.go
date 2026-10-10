//go:build !dot && !spot

package yzrs

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"time"
)

// Own only this small chain; never flush or replace upstream's TECHO5-IN policy.
// A crashed daemon can leave this chain; startup reconstructs it, teardown removes it.
func pttFirewall(pc string) (func(), error) {
	ip := net.ParseIP(pc)
	if ip == nil || ip.To4() == nil || !ip.IsPrivate() {
		return nil, errors.New("invalid PTT firewall peer")
	}
	run := func(args ...string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, "iptables-legacy", args...).Run()
	}
	return configureFirewall(run, pc)
}
func configureFirewall(run func(...string) error, pc string) (func(), error) {
	cleanup := func() {
		_ = run("-D", "TECHO5-IN", "-j", "YZRS-PTT")
		_ = run("-F", "YZRS-PTT")
		_ = run("-X", "YZRS-PTT")
	}
	_ = run("-N", "YZRS-PTT")
	if run("-F", "YZRS-PTT") != nil || run("-A", "YZRS-PTT", "-s", pc, "-p", "tcp", "--dport", "17327", "-j", "ACCEPT") != nil {
		cleanup()
		return nil, errors.New("PTT firewall unavailable")
	}
	if run("-C", "TECHO5-IN", "-j", "YZRS-PTT") != nil {
		if run("-I", "TECHO5-IN", "1", "-j", "YZRS-PTT") != nil {
			cleanup()
			return nil, errors.New("PTT firewall unavailable")
		}
	}
	return cleanup, nil
}
