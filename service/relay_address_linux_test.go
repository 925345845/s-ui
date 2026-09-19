//go:build linux

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Hhz0823/1s-ui/database"
	"github.com/Hhz0823/1s-ui/database/model"
)

func TestRelayLinuxFillLifecycle(t *testing.T) {
	if os.Getenv("SUI_RELAY_NETNS_TEST") != "1" {
		t.Skip("requires the isolated relay network namespace")
	}
	for _, stop := range []bool{false, true} {
		t.Run(fmt.Sprint("stop=", stop), func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("SUI_DB_FOLDER", dir)
			if err := database.InitDB(filepath.Join(dir, "fill.db")); err != nil {
				t.Fatal(err)
			}
			if _, err := (&SettingService{}).GetAllSetting(); err != nil {
				t.Fatal(err)
			}
			ctx, j, req := fillTestJob(t, 3)
			req.Mode, req.Protocol = relayModePaired, "socks"
			req.BaseIPv6, req.Prefix, req.Interface = "2001:db8:abcd:1234::1", 64, "relaytest0"
			req.AddSystemAddresses = true
			var mu sync.Mutex
			var added []string
			t.Cleanup(func() {
				for _, ip := range added {
					_ = deleteRelayAddress(req.Interface, ip, 64)
				}
			})
			checks := relayCreationChecks{
				add: func(ctx context.Context, iface, ip string, prefix int) error {
					mu.Lock()
					added = append(added, ip)
					mu.Unlock()
					return addRelayAddressContext(ctx, iface, ip, prefix)
				},
				ready: checkRelayRowsReady,
				ipv4:  func(context.Context, RelayUpstream) error { return nil },
			}
			rounds := 0
			var firstSaved model.RelayItem
			var poolID uint
			var firstRoundIPs []string
			runRelayFill(ctx, j, req, func(ctx context.Context, round RelayCreateRequest) (*model.RelayPool, error) {
				rounds++
				if rounds > 3 {
					t.Fatal("unexpected retry")
				}
				if rounds == 1 {
					checks.ipv6 = func(context.Context, netip.Addr) error { return errors.New("first candidates unavailable") }
				} else if rounds == 2 {
					checks.ipv6 = func(context.Context, netip.Addr) error { return nil }
					checks.ipv4 = func(_ context.Context, u RelayUpstream) error {
						if u.Server != req.Upstreams[1].Server {
							return errors.New("not ready yet")
						}
						return nil
					}
				} else {
					checks.ipv4 = func(context.Context, RelayUpstream) error { return nil }
					checks.ipv6 = func(ctx context.Context, _ netip.Addr) error {
						if stop {
							j.cancel()
							return ctx.Err()
						}
						return nil
					}
				}
				pool, err := (&ConfigService{}).createRelayContext(ctx, round, "test", "192.0.2.100", &checks)
				if rounds == 2 && pool != nil && pool.Id != 0 {
					poolID = pool.Id
					var items []model.RelayItem
					if err := json.Unmarshal(pool.Items, &items); err != nil {
						t.Fatal(err)
					}
					firstSaved = items[0]
				}
				if rounds == 1 {
					firstRoundIPs = append([]string{}, added...)
				}
				return pool, err
			}, func(context.Context) error { return nil })
			wantCount, wantStage := 3, "done"
			if stop {
				wantCount, wantStage = 1, "stopped"
			}
			if s := j.snapshot(); s.Matched != wantCount || s.Stage != wantStage {
				t.Fatalf("status=%+v", s)
			}
			var pools []model.RelayPool
			if err := database.GetDB().Find(&pools).Error; err != nil {
				t.Fatal(err)
			}
			if len(pools) != 1 || pools[0].Id != poolID || pools[0].Name != req.Name || pools[0].Count != wantCount {
				t.Fatalf("rounds must append to one pool: %+v", pools)
			}
			if len(j.snapshot().PoolIDs) != 1 {
				t.Fatal("status contains more than one pool")
			}
			var links []model.RelayRefreshLink
			if err := database.GetDB().Where("pool_id = ?", poolID).Find(&links).Error; err != nil {
				t.Fatal(err)
			}
			if len(links) != wantCount {
				t.Fatalf("refresh links=%d want=%d", len(links), wantCount)
			}
			for _, link := range links {
				if link.InboundTag == firstSaved.InboundTag && link.Token != firstSaved.RefreshToken {
					t.Fatal("existing rotation URL changed")
				}
			}
			saved := map[string]bool{}
			rows := map[int]bool{}
			for _, pool := range pools {
				var items []model.RelayItem
				if err := json.Unmarshal(pool.Items, &items); err != nil {
					t.Fatal(err)
				}
				for _, item := range items {
					if item.SourceRow == 2 && (item.InboundID != firstSaved.InboundID || item.Export != firstSaved.Export || item.Password != firstSaved.Password || item.IPv6 != firstSaved.IPv6) {
						t.Fatal("append changed an existing node")
					}
					if item.SourceRow < 1 || item.SourceRow > 3 || rows[item.SourceRow] {
						t.Fatalf("invalid original row: %+v", item)
					}
					if relayItemUpstream(item) != req.Upstreams[item.SourceRow-1] || !item.AppleIDIPv4Only {
						t.Fatalf("pair changed: %+v", item)
					}
					rows[item.SourceRow], saved[item.IPv6] = true, true
				}
			}
			if len(rows) != wantCount || !rows[2] {
				t.Fatalf("saved rows=%v", rows)
			}
			state, err := exec.Command("ip", "-6", "-o", "addr", "show", "dev", req.Interface).CombinedOutput()
			if err != nil {
				t.Fatal(err)
			}
			for _, ip := range added {
				present := relayIPv6AddressState(string(state), ip) == relayAddressReady
				if present != saved[ip] {
					t.Fatalf("failed/cancelled address retained or saved address removed: %s", ip)
				}
			}
			for _, ip := range firstRoundIPs {
				if saved[ip] {
					t.Fatal("failed candidate reused")
				}
			}
			if relayIPv6AddressState(string(state), req.BaseIPv6) != relayAddressReady {
				t.Fatal("existing base removed")
			}
		})
	}
}

func TestRelayLinuxPartialCreation(t *testing.T) {
	if os.Getenv("SUI_RELAY_NETNS_TEST") != "1" {
		t.Skip("requires the isolated relay network namespace")
	}
	if corePtr != nil {
		t.Fatal("test must not start the proxy core")
	}
	dir := t.TempDir()
	t.Setenv("SUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "partial.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := (&SettingService{}).GetAllSetting(); err != nil {
		t.Fatal(err)
	}
	const iface = "relaytest0"
	ipv6 := []string{"2001:db8:abcd:1234::301", "2001:db8:abcd:1234::302", "2001:db8:abcd:1234::303", "2001:db8:abcd:1234::304"}
	// Imported failed address belongs to the operator and survives cleanup.
	if err := addRelayAddress(iface, ipv6[1], 64); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, ip := range ipv6 {
			_ = deleteRelayAddress(iface, ip, 64)
		}
	})
	req := RelayCreateRequest{Mode: relayModePaired, Protocol: "socks", BaseIPv6: "2001:db8:abcd:1234::1", Prefix: 64, Interface: iface,
		PortStart: 30000, PasswordLength: 12, AddSystemAddresses: true, IPv6Addresses: ipv6, AppleIDIPv4Only: true,
		Upstreams: []RelayUpstream{{Server: "192.0.2.1", Port: 1080}, {Server: "192.0.2.2", Port: 1081}, {Server: "192.0.2.3", Port: 1082}, {Server: "192.0.2.4", Port: 1083}}}
	checks := relayCreationChecks{add: addRelayAddressContext, ready: checkRelayRowsReady,
		ipv6: func(_ context.Context, ip netip.Addr) error {
			if ip.String() == ipv6[1] {
				return errors.New("unreachable")
			}
			return nil
		},
		ipv4: func(_ context.Context, u RelayUpstream) error {
			if u.Server == "192.0.2.3" {
				return errors.New("authentication failed")
			}
			return nil
		},
	}
	service := &ConfigService{}
	pool, err := service.createRelayContext(context.Background(), req, "test", "192.0.2.100", &checks)
	if err != nil || pool == nil || pool.Count != 3 || pool.Id == 0 || len(pool.CreationReport.Skipped) != 2 {
		t.Fatalf("pool=%+v err=%v", pool, err)
	}
	var saved model.RelayPool
	if err := database.GetDB().First(&saved, pool.Id).Error; err != nil {
		t.Fatal(err)
	}
	var items []model.RelayItem
	if err := json.Unmarshal(saved.Items, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].SourceRow != 1 || items[1].SourceRow != 2 || items[2].SourceRow != 4 {
		t.Fatalf("saved wrong rows: %+v", items)
	}
	for _, item := range items {
		u := req.Upstreams[item.SourceRow-1]
		if item.IPv6 != ipv6[item.IPv6SourceRow-1] || relayItemUpstream(item) != u || !item.AppleIDIPv4Only {
			t.Fatalf("pair changed: %+v", item)
		}
		var outbound model.Outbound
		if err := database.GetDB().Where("tag = ?", item.IPv4OutboundTag).First(&outbound).Error; err != nil {
			t.Fatal(err)
		}
		var options map[string]interface{}
		if err := json.Unmarshal(outbound.Options, &options); err != nil {
			t.Fatal(err)
		}
		if options["server"] != u.Server || options["server_port"] != float64(u.Port) {
			t.Fatalf("actual outbound shifted: %+v", options)
		}
	}
	state, err := exec.Command("ip", "-6", "-o", "addr", "show", "dev", iface).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	for _, ip := range ipv6 {
		want := relayAddressReady
		if got := relayIPv6AddressState(string(state), ip); got != want {
			t.Fatalf("ip=%s got=%s want=%s", ip, got, want)
		}
	}
	var inboundCount int64
	database.GetDB().Model(&model.Inbound{}).Count(&inboundCount)
	if inboundCount != 3 {
		t.Fatalf("failed rows created resources: %d", inboundCount)
	}
	// All failures must not save an empty batch or any extra inbound.
	checks.ipv6 = func(context.Context, netip.Addr) error { return errors.New("unreachable") }
	failed, err := service.createRelayContext(context.Background(), req, "test", "192.0.2.100", &checks)
	if err == nil || failed == nil || failed.Id != 0 || len(failed.CreationReport.Skipped) != 4 {
		t.Fatalf("all failed: %+v err=%v", failed, err)
	}
	var pools int64
	database.GetDB().Model(&model.RelayPool{}).Count(&pools)
	if pools != 1 {
		t.Fatalf("empty pool saved: %d", pools)
	}
	// With public checks disabled, even unreachable destinations must not stop
	// any row from being saved/exported. Local address preparation still runs.
	disabled := false
	req.VerifyEgress = &disabled
	checks.ipv6 = func(context.Context, netip.Addr) error {
		t.Error("unexpected IPv6 probe")
		return errors.New("unreachable")
	}
	checks.ipv4 = func(context.Context, RelayUpstream) error {
		t.Error("unexpected IPv4 probe")
		return errors.New("unreachable")
	}
	unchecked, err := service.createRelayContext(context.Background(), req, "test", "192.0.2.100", &checks)
	if err != nil || unchecked.Count != 4 || len(unchecked.CreationReport.Skipped) != 0 {
		t.Fatalf("unchecked creation: %+v %v", unchecked, err)
	}
	var uncheckedSaved model.RelayPool
	if err := database.GetDB().First(&uncheckedSaved, unchecked.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(uncheckedSaved.Items, &items); err != nil {
		t.Fatal(err)
	}
	for i, item := range items {
		if item.SourceRow != i+1 || item.IPv6 != ipv6[i] || relayItemUpstream(item) != req.Upstreams[i] || !item.AppleIDIPv4Only || item.EgressCheck != "not_checked" || item.Export == "" {
			t.Fatalf("unchecked row lost pairing, export or status: %+v", item)
		}
	}
}

// Run only in the dedicated network namespace created by the CI script.
// This exercises real iproute2/netlink without touching the host network.
func TestRelayLinuxAddressLifecycle(t *testing.T) {
	if os.Getenv("SUI_RELAY_NETNS_TEST") != "1" {
		t.Skip("requires the isolated relay network namespace")
	}
	const iface = "relaytest0"
	const base = "2001:db8:abcd:1234::1"
	bindings := func() map[string]int {
		t.Helper()
		device, err := net.InterfaceByName(iface)
		if err != nil {
			t.Fatal(err)
		}
		addresses, err := device.Addrs()
		if err != nil {
			t.Fatal(err)
		}
		result := make(map[string]int)
		for _, address := range addresses {
			p, err := netip.ParsePrefix(address.String())
			if err == nil {
				result[p.Addr().String()] = p.Bits()
			}
		}
		return result
	}
	if bindings()[base] != 64 {
		t.Fatal("isolated base /64 is missing")
	}
	req := RelayCreateRequest{Mode: relayModePaired, Count: 1, Prefix: 64, Interface: iface, PortStart: 30000, PasswordLength: 12}
	for range 10 {
		req.Upstreams = append(req.Upstreams, RelayUpstream{Server: "192.0.2.1", Port: 1080})
	}
	items, err := (&ConfigService{}).prepareRelayItems(req)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := bindRelayAddressPool(context.Background(), items, map[string]bool{base: true}, true, addRelayAddressContext, nil)
	if err != nil || len(owned) != 10 {
		t.Fatalf("owned=%d err=%v", len(owned), err)
	}
	if err := waitRelayAddressesReady(items); err != nil {
		t.Fatal(err)
	}
	bound := bindings()
	occupied := map[string]bool{base: true}
	for _, item := range items {
		if bound[item.IPv6] != 128 || item.Prefix != 64 || !item.AddedByUs {
			t.Fatalf("binding and allocation prefixes confused: %+v bindings=%v", item, bound)
		}
		occupied[item.IPv6] = true
	}
	rotationOccupied := make(map[string]bool, len(occupied))
	for ip := range occupied {
		rotationOccupied[ip] = true
	}
	rotated, err := prepareRelayRotatedItems(items, rotationOccupied)
	if err != nil {
		t.Fatal(err)
	}
	allocation := netip.MustParsePrefix("2001:db8:abcd:1234::/64")
	for _, item := range rotated {
		if item.Prefix != 64 || !allocation.Contains(netip.MustParseAddr(item.IPv6)) || occupied[item.IPv6] {
			t.Fatalf("rotation lost allocation: %+v", item)
		}
	}
	// Simulate rollback of a newly bound pool.
	for _, item := range owned {
		if err := deleteRelayAddress(item.Interface, item.IPv6, item.Prefix); err != nil {
			t.Fatal(err)
		}
	}
	bound = bindings()
	for _, item := range owned {
		if _, exists := bound[item.IPv6]; exists {
			t.Fatalf("rollback left %s behind", item.IPv6)
		}
	}
	if bound[base] != 64 {
		t.Fatal("rollback changed existing base")
	}
	// The startup/periodic restoration path uses this same address helper.
	item := owned[0]
	if err := addRelayAddress(item.Interface, item.IPv6, item.Prefix); err != nil {
		t.Fatal(err)
	}
	if bindings()[item.IPv6] != 128 {
		t.Fatal("restoration did not use a host binding")
	}
	if err := deleteRelayAddress(item.Interface, item.IPv6, item.Prefix); err != nil {
		t.Fatal(err)
	}
	if err := deleteRelayAddress(item.Interface, item.IPv6, item.Prefix); err != nil {
		t.Fatalf("repeated delete: %v", err)
	}
	// A persisted pre-patch pool may still have an actual /64 binding.
	legacy := model.RelayItem{Interface: iface, IPv6: "2001:db8:abcd:1234::abcd", Prefix: 64}
	if output, err := exec.Command("ip", "-6", "addr", "add", legacy.IPv6+"/64", "dev", iface, "nodad").CombinedOutput(); err != nil {
		t.Fatalf("legacy setup: %v %s", err, output)
	}
	if err := deleteRelayAddress(iface, legacy.IPv6, legacy.Prefix); err != nil {
		t.Fatal(err)
	}
	bound = bindings()
	if _, exists := bound[legacy.IPv6]; exists {
		t.Fatal("legacy /64 binding was not removed")
	}
	if bound[base] != 64 {
		t.Fatal("legacy cleanup changed base")
	}
}
