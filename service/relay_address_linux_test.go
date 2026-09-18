//go:build linux

package service

import (
	"context"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"testing"

	"github.com/Hhz0823/1s-ui/database/model"
)

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
