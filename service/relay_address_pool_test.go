package service

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Hhz0823/1s-ui/database/model"
)

func TestRelayGeneratedPoolMatchesUpstreamRows(t *testing.T) {
	for _, mode := range []string{relayModePaired, relayModeDualStack} {
		t.Run(mode, func(t *testing.T) {
			req := RelayCreateRequest{Mode: mode, Count: 3, BaseIPv6: "2001:db8:abcd:1234::1", Prefix: 64, Interface: "eth0", PortStart: 30000, PasswordLength: 12}
			for i := range 100 {
				req.Upstreams = append(req.Upstreams, RelayUpstream{Server: "192.0.2.1", Port: 1080, Username: fmt.Sprint(i), Password: "secret"})
			}
			items, err := (&ConfigService{}).prepareRelayItems(req)
			if err != nil || len(items) != len(req.Upstreams) {
				t.Fatalf("generated %d rows, err=%v", len(items), err)
			}
			prefix := netip.MustParsePrefix("2001:db8:abcd:1234::/64")
			seen := make(map[string]bool)
			for i, item := range items {
				ip, err := netip.ParseAddr(item.IPv6)
				if err != nil || !prefix.Contains(ip) || item.IPv6 == req.BaseIPv6 || seen[item.IPv6] {
					t.Fatalf("invalid or duplicate address at row %d: %s", i+1, item.IPv6)
				}
				seen[item.IPv6] = true
				u := req.Upstreams[i]
				if item.UpstreamServer != u.Server || item.UpstreamPort != u.Port || item.UpstreamUsername != u.Username || item.UpstreamPassword != u.Password || item.ListenPort != req.PortStart+i {
					t.Fatalf("pairing changed at row %d", i+1)
				}
			}
		})
	}
}

func TestRelayPoolBindingCompletesAllRowsWithBoundedConcurrency(t *testing.T) {
	items := make([]model.RelayItem, 100)
	for i := range items {
		items[i] = model.RelayItem{IPv6: fmt.Sprintf("2001:db8::%x", i+1), Interface: "eth0", Prefix: 64, UpstreamUsername: fmt.Sprint(i)}
	}
	var active, peak atomic.Int32
	last := 0
	owned, err := bindRelayAddressPool(context.Background(), items, map[string]bool{items[0].IPv6: true}, true,
		func(_ context.Context, iface, ip string, prefix int) error {
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old; old = peak.Load() {
				if peak.CompareAndSwap(old, n) {
					break
				}
			}
			if iface != "eth0" || prefix != 64 || ip == items[0].IPv6 {
				t.Error("incorrect binding arguments")
			}
			time.Sleep(time.Millisecond)
			return nil
		}, func(done, total int) {
			if done != last+1 || total != 99 {
				t.Errorf("bad progress %d/%d after %d", done, total, last)
			}
			last = done
		})
	if err != nil || len(owned) != 99 || last != 99 || active.Load() != 0 || peak.Load() > 4 {
		t.Fatalf("owned=%d progress=%d peak=%d active=%d err=%v", len(owned), last, peak.Load(), active.Load(), err)
	}
	for i, item := range items {
		if item.AddedByUs != (i > 0) || item.UpstreamUsername != fmt.Sprint(i) {
			t.Fatalf("ownership or order changed at row %d", i)
		}
	}
}

func TestRelayPoolBindingFailureTracksSuccessfulInFlightAdds(t *testing.T) {
	items := []model.RelayItem{{IPv6: "2001:db8::1"}, {IPv6: "2001:db8::2"}, {IPv6: "2001:db8::3"}, {IPv6: "2001:db8::4"}, {IPv6: "2001:db8::5"}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan struct{}, 4)
	bindErr := errors.New("binding failed")
	owned, err := bindRelayAddressPool(ctx, items, map[string]bool{items[0].IPv6: true}, true,
		func(ctx context.Context, _, ip string, _ int) error {
			started <- struct{}{}
			if ip == items[1].IPv6 {
				for range 4 {
					select {
					case <-started:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return bindErr
			}
			// These system calls completed successfully just as another call failed.
			<-ctx.Done()
			return nil
		}, nil)
	if !errors.Is(err, bindErr) || len(owned) != 3 {
		t.Fatalf("owned=%d err=%v", len(owned), err)
	}
	for _, item := range owned {
		if !item.AddedByUs || item.IPv6 == items[0].IPv6 || item.IPv6 == items[1].IPv6 {
			t.Fatalf("wrong rollback ownership: %+v", item)
		}
	}
}

func TestRelayPoolBindingDisabledPreflightsWithoutMutation(t *testing.T) {
	items := []model.RelayItem{{IPv6: "2001:db8::1"}, {IPv6: "2001:db8::2"}}
	owned, err := bindRelayAddressPool(context.Background(), items, map[string]bool{items[0].IPv6: true}, false,
		func(context.Context, string, string, int) error { t.Error("unexpected mutation"); return nil }, nil)
	if err == nil || len(owned) != 0 {
		t.Fatalf("owned=%v err=%v", owned, err)
	}
}

func TestRelayPoolSettleHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitRelayPoolSettle(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestRelayDetectedBaseHonorsAllocationPrefix(t *testing.T) {
	detected := []RelayIPv6{
		{Interface: "eth1", Address: "2001:db8:ffff::1", Prefix: 64},
		{Interface: "eth0", Address: "2001:db8:1234:5678::2", Prefix: 128},
		{Interface: "eth0", Address: "2001:db8:1234:5678::1", Prefix: 64},
	}
	req := RelayCreateRequest{Interface: "eth0", Prefix: 64, Count: 100}
	base, prefix, iface, err := resolveDetectedRelayBase(req, detected)
	if err != nil || prefix != 64 || iface != "eth0" || base.String() != detected[1].Address {
		t.Fatalf("base=%s prefix=%d iface=%s err=%v", base, prefix, iface, err)
	}
	req.BaseIPv6 = base.String()
	want, wantPrefix, wantIface, err := resolveRelayBase(req)
	if err != nil || base != want || prefix != wantPrefix || iface != wantIface {
		t.Fatalf("auto and explicit selection differ: %v", err)
	}
	for _, invalidPrefix := range []int{-1, 127, 128, 129} {
		req.BaseIPv6 = ""
		req.Prefix = invalidPrefix
		if _, _, _, err := resolveDetectedRelayBase(req, detected); err == nil {
			t.Fatalf("accepted invalid/insufficient prefix %d for 100 rows", invalidPrefix)
		}
	}
}
