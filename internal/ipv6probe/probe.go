// Package ipv6probe checks Internet reachability using an explicitly bound IPv6.
package ipv6probe

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

const connectTimeout = 5 * time.Second

var targets = []string{
	"[2606:4700:4700::1111]:443",
	"[2001:4860:4860::8888]:443",
}

// Probe waits at most 18 seconds (excluding scheduling overhead). A newly
// assigned address can finish local DAD before the upstream network learns it.
// Retry failed rounds without bypassing the source-bound reachability check.
func Probe(ctx context.Context, address netip.Addr) error {
	return probe(ctx, address, targets, connectTimeout,
		[]time.Duration{0, time.Second, 2 * time.Second}, dial)
}

// Creation gets longer connection attempts and settling intervals (32 seconds
// maximum per address). Rotation retains its existing shorter probe policy.
func ProbeForCreation(ctx context.Context, address netip.Addr) error {
	return probe(ctx, address, targets, 8*time.Second,
		[]time.Duration{0, 3 * time.Second, 5 * time.Second}, dial)
}

func dial(ctx context.Context, address netip.Addr, target string) error {
	dialer := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.IP(address.AsSlice())}}
	connection, err := dialer.DialContext(ctx, "tcp6", target)
	if err != nil {
		return err
	}
	_ = connection.Close()
	return nil
}

func probe(ctx context.Context, address netip.Addr, endpoints []string, timeout time.Duration,
	delays []time.Duration, connect func(context.Context, netip.Addr, string) error) error {
	if !address.Is6() || address.Is4In6() {
		return fmt.Errorf("IPv6 egress probe requires an IPv6 address")
	}
	var failures []string
	for round, delay := range delays {
		if err := ctx.Err(); err != nil {
			return err
		}
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		var err error
		failures, err = probeRound(ctx, address, endpoints, timeout, connect)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if round == len(delays)-1 {
			return fmt.Errorf("IPv6 %s failed TCP egress checks after %d rounds (%s per target): %s",
				address, len(delays), timeout, strings.Join(failures, "; "))
		}
	}
	return fmt.Errorf("IPv6 egress probe has no attempts configured")
}

// Both targets race under the same deadline. A blackholed first target must
// not delay a healthy second target or occupy a batch worker for five seconds.
func probeRound(ctx context.Context, address netip.Addr, endpoints []string, timeout time.Duration,
	connect func(context.Context, netip.Addr, string) error) ([]string, error) {
	roundContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type result struct {
		index int
		err   error
	}
	results := make(chan result, len(endpoints))
	for index, target := range endpoints {
		go func() { results <- result{index, connect(roundContext, address, target)} }()
	}
	failures := make([]string, len(endpoints))
	for range endpoints {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case r := <-results:
			if r.err == nil {
				return nil, nil
			}
			failures[r.index] = fmt.Sprintf("%s: %v", endpoints[r.index], r.err)
		}
	}
	return failures, fmt.Errorf("all targets failed")
}
