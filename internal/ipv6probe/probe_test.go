package ipv6probe

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestProbeRecoversAfterInitialTimeouts(t *testing.T) {
	address := netip.MustParseAddr("2001:db8::2")
	calls := 0
	var contexts []context.Context
	err := probe(context.Background(), address, targets, connectTimeout, []time.Duration{0, 0, 0},
		func(ctx context.Context, source netip.Addr, target string) error {
			contexts = append(contexts, ctx)
			if source != address || target != targets[calls%len(targets)] {
				t.Fatalf("wrong source/target: %s -> %s", source, target)
			}
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > connectTimeout || time.Until(deadline) <= 0 {
				t.Fatal("missing or invalid per-target deadline")
			}
			calls++
			if calls <= 2 {
				return context.DeadlineExceeded
			}
			return nil
		})
	if err != nil || calls != 3 {
		t.Fatalf("calls=%d err=%v; expected recovery on second round", calls, err)
	}
	for _, ctx := range contexts {
		if ctx.Err() == nil {
			t.Fatal("attempt context was not released")
		}
	}
}

func TestProbeAlternateTargetSucceedsWithoutRetry(t *testing.T) {
	calls := 0
	err := probe(context.Background(), netip.MustParseAddr("2001:db8::2"), targets,
		connectTimeout, []time.Duration{0, time.Hour}, func(context.Context, netip.Addr, string) error {
			calls++
			if calls == 1 {
				return errors.New("first target unavailable")
			}
			return nil
		})
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestProbePermanentFailureIncludesBothTargets(t *testing.T) {
	calls := 0
	err := probe(context.Background(), netip.MustParseAddr("2001:db8::2"), targets,
		connectTimeout, []time.Duration{0, 0, 0}, func(context.Context, netip.Addr, string) error {
			calls++
			return errors.New("i/o timeout")
		})
	if err == nil || calls != 6 {
		t.Fatalf("calls=%d err=%v; expected bounded failure", calls, err)
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
	calls := 0
	err := probe(ctx, netip.MustParseAddr("2001:db8::2"), targets, connectTimeout,
		[]time.Duration{0, 0, 0}, func(ctx context.Context, _ netip.Addr, _ string) error {
			calls++
			<-ctx.Done()
			return ctx.Err()
		})
	if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
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
