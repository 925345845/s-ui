package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Hhz0823/1s-ui/database/model"
)

func TestRelayCreationProgressIsScopedAndTerminal(t *testing.T) {
	beginRelayProgress("alice", "request-one")
	setRelayProgress("ipv6", 3, 10)
	if GetRelayCreateProgress("bob", "request-one") != nil || GetRelayCreateProgress("alice", "request-two") != nil {
		t.Fatal("progress leaked across actors or requests")
	}
	p := GetRelayCreateProgress("alice", "request-one")
	if p == nil || !p.Active || p.Completed != 3 || p.Total != 10 {
		t.Fatalf("bad progress: %+v", p)
	}
	finishRelayProgress(42, nil)
	p = GetRelayCreateProgress("alice", "request-one")
	if p.Active || p.Stage != "done" || p.PoolID != 42 {
		t.Fatalf("bad completion: %+v", p)
	}
	beginRelayProgress("alice", "request-two")
	finishRelayProgress(0, errors.New("failed"))
	if GetRelayCreateProgress("alice", "request-one") != nil {
		t.Fatal("stale request still visible")
	}
	if p = GetRelayCreateProgress("alice", "request-two"); p.Active || p.Stage != "failed" {
		t.Fatalf("bad failure: %+v", p)
	}
}

func TestCreateRelayRejectsBusyWithoutQueuing(t *testing.T) {
	relayMu.Lock()
	defer relayMu.Unlock()
	_, err := (&ConfigService{}).CreateRelay(RelayCreateRequest{}, "test", "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("unexpected result: %v", err)
	}
}

func TestValidateRelayIPv6EgressCancelledBatchCannotSucceed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := validateRelayIPv6Egress(ctx, []model.RelayItem{{IPv6: "2001:db8::1"}}, func(context.Context, netip.Addr) error {
		t.Error("probe started despite cancellation")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation became success: %v", err)
	}
}

func TestValidateRelayIPv6EgressBatchDeadlineStopsQueuedWork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	items := make([]model.RelayItem, 100)
	for i := range items {
		items[i].IPv6 = fmt.Sprintf("2001:db8::%x", i+1)
	}
	var calls atomic.Int32
	err := validateRelayIPv6Egress(ctx, items, func(ctx context.Context, _ netip.Addr) error {
		calls.Add(1)
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() > relayIPv6ProbeWorkers {
		t.Fatalf("calls=%d, error=%v", calls.Load(), err)
	}
}

func TestValidateRelayIPv6EgressProgressCountsUniqueSuccesses(t *testing.T) {
	items := []model.RelayItem{{IPv6: "2001:db8::1"}, {IPv6: "2001:db8::2"}, {IPv6: "2001:db8::1"}}
	last := 0
	err := validateRelayIPv6EgressProgress(context.Background(), items, func(context.Context, netip.Addr) error { return nil }, func(done, total int) {
		if done != last+1 || total != 2 {
			t.Errorf("bad progress: %d/%d after %d", done, total, last)
		}
		last = done
	})
	if err != nil || last != 2 {
		t.Fatalf("last=%d err=%v", last, err)
	}
}

func TestRelayIPv4ChecksEveryCredentialAndReportsLine(t *testing.T) {
	upstreams := make([]RelayUpstream, 100)
	for i := range upstreams {
		upstreams[i] = RelayUpstream{Server: "proxy.test", Port: 1080, Username: fmt.Sprint(i)}
	}
	var mu sync.Mutex
	seen := make(map[string]int)
	last := 0
	err := validateRelayIPv4Upstreams(context.Background(), upstreams, func(_ context.Context, u RelayUpstream) error {
		mu.Lock()
		seen[u.Username]++
		mu.Unlock()
		return nil
	}, func(done, total int) {
		if total != 100 || done != last+1 {
			t.Errorf("bad progress: %d/%d", done, total)
		}
		last = done
	})
	if err != nil || last != 100 || len(seen) != 100 {
		t.Fatalf("last=%d checked=%d err=%v", last, len(seen), err)
	}
	for _, count := range seen {
		if count != 1 {
			t.Fatal("duplicate credential check")
		}
	}
	err = validateRelayIPv4Upstreams(context.Background(), upstreams, func(_ context.Context, u RelayUpstream) error {
		if u.Username == "7" {
			return errors.New("authentication failed")
		}
		return nil
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "line 8") {
		t.Fatalf("wrong row: %v", err)
	}
}

func TestRelayIPv4CancelledBatchCannotSucceed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := validateRelayIPv4Upstreams(ctx, []RelayUpstream{{Server: "proxy.test", Port: 1080}}, func(context.Context, RelayUpstream) error { t.Error("unexpected dial"); return nil }, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}

func TestRelaySOCKS5AuthenticatesAndRequestsIPv4(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(fmt.Sprintf("reject=%v", reject), func(t *testing.T) {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			serverDone := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					serverDone <- err
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				serverDone <- serveRelayTestSOCKS(conn, reject)
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err = probeRelaySOCKS5(ctx, RelayUpstream{Server: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, Username: "test-user", Password: "test-secret"}, []string{"1.1.1.1:443"})
			if (err != nil) != reject {
				t.Fatalf("probe error=%v", err)
			}
			if err != nil && (strings.Contains(err.Error(), "test-secret") || strings.Contains(err.Error(), "test-user")) {
				t.Fatal("credentials leaked")
			}
			if err := <-serverDone; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func serveRelayTestSOCKS(conn net.Conn, reject bool) error {
	read := func(n int) ([]byte, error) { b := make([]byte, n); _, err := io.ReadFull(conn, b); return b, err }
	header, err := read(2)
	if err != nil {
		return err
	}
	if header[0] != 5 {
		return errors.New("not SOCKS5")
	}
	if _, err = read(int(header[1])); err != nil {
		return err
	}
	if _, err = conn.Write([]byte{5, 2}); err != nil {
		return err
	}
	auth, err := read(2)
	if err != nil {
		return err
	}
	user, err := read(int(auth[1]))
	if err != nil {
		return err
	}
	n, err := read(1)
	if err != nil {
		return err
	}
	pass, err := read(int(n[0]))
	if err != nil {
		return err
	}
	if string(user) != "test-user" || string(pass) != "test-secret" {
		return errors.New("wrong authentication")
	}
	if _, err = conn.Write([]byte{1, 0}); err != nil {
		return err
	}
	request, err := read(10)
	if err != nil {
		return err
	}
	if request[0] != 5 || request[1] != 1 || request[3] != 1 || !net.IP(request[4:8]).Equal(net.ParseIP("1.1.1.1")) || request[8] != 1 || request[9] != 187 {
		return fmt.Errorf("not IPv4 CONNECT to port 443: %v", request)
	}
	status := byte(0)
	if reject {
		status = 5
	}
	_, err = conn.Write([]byte{5, status, 0, 1, 127, 0, 0, 1, 0, 80})
	return err
}

func TestRelayPairingPreservesEachUpstream(t *testing.T) {
	req := RelayCreateRequest{Mode: relayModePaired, Count: 2, BaseIPv6: "2001:db8:abcd:1234::1", Prefix: 64, Interface: "eth0", PortStart: 30000,
		IPv6Addresses: []string{"2001:db8:abcd:1234::2", "2001:db8:abcd:1234::3"},
		Upstreams:     []RelayUpstream{{Server: "proxy-a.test", Port: 1080, Username: "a", Password: "one"}, {Server: "proxy-b.test", Port: 1081, Username: "b", Password: "two"}}}
	items, err := (&ConfigService{}).prepareRelayItems(req)
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range items {
		u := req.Upstreams[i]
		if item.IPv6 != req.IPv6Addresses[i] || item.UpstreamServer != u.Server || item.UpstreamPort != u.Port || item.UpstreamUsername != u.Username || item.UpstreamPassword != u.Password {
			t.Fatalf("pair %d was changed", i+1)
		}
	}
}
