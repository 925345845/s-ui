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

// Probe waits at most 33 seconds (excluding scheduling overhead). A newly
// assigned address can finish local DAD before the upstream network learns it.
// Retry failed rounds without bypassing the source-bound reachability check.
func Probe(ctx context.Context, address netip.Addr) error {
	return probe(ctx, address, targets, connectTimeout,
		[]time.Duration{0, time.Second, 2 * time.Second}, dial)
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
		failures = failures[:0]
		for _, target := range endpoints {
			if err := ctx.Err(); err != nil {
				return err
			}
			attemptContext, cancel := context.WithTimeout(ctx, timeout)
			err := connect(attemptContext, address, target)
			cancel()
			if err == nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failures = append(failures, fmt.Sprintf("%s: %v", target, err))
		}
		if round == len(delays)-1 {
			return fmt.Errorf("IPv6 %s failed TCP egress checks after %d rounds (%s per target): %s",
				address, len(delays), timeout, strings.Join(failures, "; "))
		}
	}
	return fmt.Errorf("IPv6 egress probe has no attempts configured")
}
