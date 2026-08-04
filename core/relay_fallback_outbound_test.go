package core

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	sbOutbound "github.com/sagernet/sing-box/adapter/outbound"
	C "github.com/sagernet/sing-box/constant"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type relayFallbackTestOutbound struct {
	sbOutbound.Adapter
	err         error
	calls       int
	destination M.Socksaddr
}

func newRelayFallbackTestOutbound(tag string, err error) *relayFallbackTestOutbound {
	return &relayFallbackTestOutbound{
		Adapter: sbOutbound.NewAdapter("test", tag, []string{N.NetworkTCP, N.NetworkUDP}, nil),
		err:     err,
	}
}

func (o *relayFallbackTestOutbound) DialContext(_ context.Context, _ string, destination M.Socksaddr) (net.Conn, error) {
	o.calls++
	o.destination = destination
	if o.err != nil {
		return nil, o.err
	}
	client, server := net.Pipe()
	_ = server.Close()
	return client, nil
}

func (o *relayFallbackTestOutbound) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	return nil, errors.New("not implemented")
}

func TestRelayFallbackPrefersIPv6(t *testing.T) {
	ipv6 := newRelayFallbackTestOutbound("ipv6", nil)
	ipv4 := newRelayFallbackTestOutbound("ipv4", nil)
	outbound := &relayFallbackOutbound{ipv6Outbound: ipv6, ipv4Outbound: ipv4, ipv6Timeout: time.Second}
	destination := M.ParseSocksaddr("example.com:443")
	conn, err := outbound.DialParallelNetwork(context.Background(), N.NetworkTCP, destination, []netip.Addr{
		netip.MustParseAddr("2001:db8::10"),
		netip.MustParseAddr("192.0.2.10"),
	}, nil, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if ipv6.calls != 1 || ipv4.calls != 0 {
		t.Fatalf("calls: IPv6=%d IPv4=%d", ipv6.calls, ipv4.calls)
	}
}

func TestRelayFallbackUsesIPv4SOCKSAfterIPv6Failure(t *testing.T) {
	ipv6 := newRelayFallbackTestOutbound("ipv6", errors.New("IPv6 unreachable"))
	ipv4 := newRelayFallbackTestOutbound("ipv4", nil)
	outbound := &relayFallbackOutbound{ipv6Outbound: ipv6, ipv4Outbound: ipv4, ipv6Timeout: time.Second}
	destination := M.ParseSocksaddr("appleid.apple.com:443")
	conn, err := outbound.DialParallelNetwork(context.Background(), N.NetworkTCP, destination, []netip.Addr{
		netip.MustParseAddr("2001:db8::20"),
		netip.MustParseAddr("192.0.2.20"),
	}, nil, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if ipv6.calls != 1+len(relayFallbackIPv6ProbeTargets) || ipv4.calls != 1 {
		t.Fatalf("calls: IPv6=%d IPv4=%d", ipv6.calls, ipv4.calls)
	}
	if ipv4.destination.String() != destination.String() {
		t.Fatalf("IPv4 fallback destination = %s, want %s", ipv4.destination, destination)
	}
}

func TestRelayFallbackSuppressesIPv4OnlyProbeWhileIPv6Healthy(t *testing.T) {
	ipv6 := newRelayFallbackTestOutbound("ipv6", nil)
	ipv4 := newRelayFallbackTestOutbound("ipv4", nil)
	outbound := &relayFallbackOutbound{ipv6Outbound: ipv6, ipv4Outbound: ipv4, ipv6Timeout: time.Second}
	outbound.markIPv6Healthy()
	_, err := outbound.DialParallelNetwork(context.Background(), N.NetworkTCP, M.ParseSocksaddr("v4-check.example:443"), []netip.Addr{
		netip.MustParseAddr("192.0.2.40"),
	}, nil, nil, nil, 0)
	if err == nil {
		t.Fatal("expected IPv4-only probe to be rejected while IPv6 is healthy")
	}
	if ipv6.calls != 0 || ipv4.calls != 0 {
		t.Fatalf("calls: IPv6=%d IPv4=%d", ipv6.calls, ipv4.calls)
	}
}

func TestRelayFallbackProbesIPv6BeforeDirectIPv4(t *testing.T) {
	ipv6 := newRelayFallbackTestOutbound("ipv6", nil)
	ipv4 := newRelayFallbackTestOutbound("ipv4", nil)
	outbound := &relayFallbackOutbound{ipv6Outbound: ipv6, ipv4Outbound: ipv4, ipv6Timeout: time.Second}
	_, err := outbound.DialContext(context.Background(), N.NetworkTCP, M.ParseSocksaddr("192.0.2.50:443"))
	if err == nil {
		t.Fatal("expected direct IPv4 to be rejected after a successful IPv6 health probe")
	}
	if ipv6.calls != 1 || ipv4.calls != 0 {
		t.Fatalf("calls: IPv6=%d IPv4=%d", ipv6.calls, ipv4.calls)
	}
}

func TestRelayFallbackDoesNotSendIPv6OnlyDestinationToIPv4(t *testing.T) {
	ipv6 := newRelayFallbackTestOutbound("ipv6", errors.New("IPv6 unreachable"))
	ipv4 := newRelayFallbackTestOutbound("ipv4", nil)
	outbound := &relayFallbackOutbound{ipv6Outbound: ipv6, ipv4Outbound: ipv4, ipv6Timeout: time.Second}
	_, err := outbound.DialParallelNetwork(context.Background(), N.NetworkTCP, M.ParseSocksaddr("ipv6.example:443"), []netip.Addr{
		netip.MustParseAddr("2001:db8::30"),
	}, (*C.NetworkStrategy)(nil), nil, nil, 0)
	if err == nil {
		t.Fatal("expected IPv6-only destination to fail")
	}
	if ipv4.calls != 0 {
		t.Fatalf("IPv4 fallback calls = %d, want 0", ipv4.calls)
	}
}

func TestCoreAcceptsRelayFallbackOutbound(t *testing.T) {
	instance := NewCore()
	config := []byte(`{
  "log": {"disabled": true},
  "outbounds": [
    {"type": "direct", "tag": "relay-v6", "domain_strategy": "ipv6_only"},
    {"type": "socks", "tag": "relay-v4", "server": "127.0.0.1", "server_port": 9, "version": "5"},
    {"type": "relay-fallback", "tag": "relay-dual", "ipv6_outbound": "relay-v6", "ipv4_outbound": "relay-v4", "ipv6_timeout": "3s"}
  ],
  "route": {"final": "relay-dual"}
}`)
	if err := instance.Start(config); err != nil {
		t.Fatal(err)
	}
	if err := instance.Stop(); err != nil {
		t.Fatal(err)
	}
}
