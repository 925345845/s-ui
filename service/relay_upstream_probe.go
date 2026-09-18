package service

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

// Authenticate and CONNECT to an IPv4 literal through each paired SOCKS5.
// A successful IPv6 probe alone says nothing about the paired IPv4 upstream.
func probeRelaySOCKS5(ctx context.Context, upstream RelayUpstream, targets []string) error {
	var auth *proxy.Auth
	if upstream.Username != "" || upstream.Password != "" {
		auth = &proxy.Auth{User: upstream.Username, Password: upstream.Password}
	}
	d, err := proxy.SOCKS5("tcp", net.JoinHostPort(upstream.Server, strconv.Itoa(upstream.Port)), auth, &net.Dialer{})
	if err != nil {
		return fmt.Errorf("invalid SOCKS5 endpoint")
	}
	dialer, ok := d.(proxy.ContextDialer)
	if !ok {
		return fmt.Errorf("SOCKS5 dialer does not support cancellation")
	}
	var failures []string
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
		conn, err := dialer.DialContext(attempt, "tcp", target)
		cancel()
		if err == nil {
			_ = conn.Close()
			return nil
		}
		// Never expose credentials, even if a dialer embeds them in an error.
		detail := err.Error()
		for _, secret := range []string{upstream.Password, upstream.Username} {
			if secret != "" {
				detail = strings.ReplaceAll(detail, secret, "[redacted]")
			}
		}
		failures = append(failures, target+": "+detail)
	}
	return fmt.Errorf("SOCKS5 IPv4 CONNECT failed: %s", strings.Join(failures, "; "))
}

func validateRelayIPv4Upstreams(ctx context.Context, upstreams []RelayUpstream,
	probe func(context.Context, RelayUpstream) error, progress func(int, int)) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	jobs := make(chan int, len(upstreams))
	for i := range upstreams {
		jobs <- i
	}
	close(jobs)
	var wg sync.WaitGroup
	var mu sync.Mutex
	completed := 0
	var failure error
	for range min(8, len(upstreams)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				err := probe(ctx, upstreams[i])
				mu.Lock()
				if err != nil {
					if failure == nil {
						failure = fmt.Errorf("IPv4 SOCKS5 upstream line %d: %w", i+1, err)
						cancel()
					}
					mu.Unlock()
					return
				}
				completed++
				if progress != nil {
					progress(completed, len(upstreams))
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if failure != nil {
		return failure
	}
	if ctx.Err() != nil {
		return fmt.Errorf("IPv4 batch check stopped (%d/%d passed): %w", completed, len(upstreams), ctx.Err())
	}
	return nil
}
