package ipv6probe

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProbeRecoversAfterInitialTimeouts(t *testing.T) {
	address := netip.MustParseAddr("2001:db8::2")
	var mu sync.Mutex
	calls := make(map[string]int)
	err := probe(context.Background(), address, targets, connectTimeout, []time.Duration{0, 0, 0},
		func(ctx context.Context, source netip.Addr, target string) error {
			mu.Lock()
			defer mu.Unlock()
			if source != address {
				t.Errorf("wrong source: %s", source)
			}
			if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > connectTimeout {
				t.Error("missing per-target deadline")
			}
			calls[target]++
			if calls[target] == 1 {
				return context.DeadlineExceeded
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls[targets[0]] < 2 && calls[targets[1]] < 2 {
		t.Fatal("did not retry failed round")
	}
}

func TestProbeHealthyTargetDoesNotWaitForBlackholedTarget(t *testing.T) {
	blocked := make(chan struct{})
	stopped := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := probe(ctx, netip.MustParseAddr("2001:db8::2"), targets, connectTimeout,
		[]time.Duration{0, time.Hour}, func(ctx context.Context, _ netip.Addr, target string) error {
			if target == targets[0] {
				close(blocked)
				<-ctx.Done()
				close(stopped)
				return ctx.Err()
			}
			<-blocked
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("losing dial was not cancelled")
	}
}

func TestProbePermanentFailureIncludesBothTargets(t *testing.T) {
	var calls atomic.Int32
	err := probe(context.Background(), netip.MustParseAddr("2001:db8::2"), targets,
		connectTimeout, []time.Duration{0, 0, 0}, func(context.Context, netip.Addr, string) error {
			calls.Add(1)
			return errors.New("i/o timeout")
		})
	if err == nil || calls.Load() != 6 {
		t.Fatalf("calls=%d err=%v; expected bounded failure", calls.Load(), err)
	}
	for _, detail := range []string{targets[0], targets[1], "3 rounds", "5s per target", "i/o timeout"} {
		if !strings.Contains(err.Error(), detail) {
			t.Errorf("missing %q in %v", detail, err)
		}
	}
}

func TestProbeHonorsCancellationDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- probe(ctx, netip.MustParseAddr("2001:db8::2"), targets[:1],
			connectTimeout, []time.Duration{0, time.Hour}, func(context.Context, netip.Addr, string) error {
				close(called)
				return errors.New("unavailable")
			})
	}()
	<-called
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("backoff ignored cancellation")
	}
}

func TestProbeHonorsParentDeadlineDuringDial(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var calls atomic.Int32
	err := probe(ctx, netip.MustParseAddr("2001:db8::2"), targets, connectTimeout,
		[]time.Duration{0, 0, 0}, func(ctx context.Context, _ netip.Addr, _ string) error {
			calls.Add(1)
			<-ctx.Done()
			return ctx.Err()
		})
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() < 1 || calls.Load() > 2 {
		t.Fatalf("calls=%d err=%v", calls.Load(), err)
	}
}

func TestProbeRejectsNonIPv6(t *testing.T) {
	for _, address := range []netip.Addr{{}, netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("::ffff:192.0.2.1")} {
		if err := Probe(context.Background(), address); err == nil {
			t.Errorf("accepted %s", address)
		}
	}
}

func TestDialBindsIPv6Source(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dial(ctx, netip.MustParseAddr("::1"), listener.Addr().String()); err != nil {
		t.Fatal(err)
	}
	connection, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if !connection.RemoteAddr().(*net.TCPAddr).IP.Equal(net.ParseIP("::1")) {
		t.Fatalf("unexpected source: %v", connection.RemoteAddr())
	}
}
