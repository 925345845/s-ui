package core

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	M "github.com/sagernet/sing/common/metadata"
)

type vintedStrictTestDialer struct {
	destination M.Socksaddr
}

func (d *vintedStrictTestDialer) DialContext(context.Context, string, M.Socksaddr) (net.Conn, error) {
	return nil, nil
}

func (d *vintedStrictTestDialer) ListenPacket(_ context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	d.destination = destination
	return &vintedStrictTestPacketConn{}, nil
}

type vintedStrictTestPacketConn struct{}

func (*vintedStrictTestPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, nil, net.ErrClosed
}
func (*vintedStrictTestPacketConn) WriteTo([]byte, net.Addr) (int, error) { return 0, net.ErrClosed }
func (*vintedStrictTestPacketConn) Close() error                          { return nil }
func (*vintedStrictTestPacketConn) LocalAddr() net.Addr                   { return nil }
func (*vintedStrictTestPacketConn) SetDeadline(time.Time) error           { return nil }
func (*vintedStrictTestPacketConn) SetReadDeadline(time.Time) error       { return nil }
func (*vintedStrictTestPacketConn) SetWriteDeadline(time.Time) error      { return nil }

func newVintedStrictTestOutbound() *vintedStrictOutbound {
	return &vintedStrictOutbound{
		allowedDomains:        normalizedDomainSet([]string{"api.www.vinted.com", "assets.example-cdn.test"}),
		allowedDomainSuffixes: normalizedDomains([]string{"vinted.com", "vinted.net"}),
		ipv4Domains:           normalizedDomainSet([]string{"dd.vinted.lt", "metrics-ingress-web.svc.vinted.com"}),
		ipv4DomainSuffixes:    normalizedDomains([]string{"datadome.co", "captcha-delivery.com"}),
	}
}

func TestVintedStrictOutboundRebuildsSniffedIPv4Destination(t *testing.T) {
	outbound := newVintedStrictTestOutbound()
	ctx := adapter.WithContext(context.Background(), &adapter.InboundContext{Domain: "API.WWW.VINTED.COM."})
	destination, err := outbound.domainDestination(ctx, M.SocksaddrFrom(netip.MustParseAddr("198.51.100.10"), 443))
	if err != nil {
		t.Fatal(err)
	}
	if !destination.IsDomain() || destination.Fqdn != "api.www.vinted.com" || destination.Port != 443 {
		t.Fatalf("rebuilt destination = %#v", destination)
	}
}

func TestVintedStrictOutboundRejectsUnsniffedIPv4Literal(t *testing.T) {
	outbound := newVintedStrictTestOutbound()
	_, err := outbound.domainDestination(context.Background(), M.SocksaddrFrom(netip.MustParseAddr("198.51.100.10"), 443))
	if err == nil {
		t.Fatal("unsniffed IPv4 literal was accepted")
	}
}

func TestVintedStrictOutboundRejectsIPv4SessionDomain(t *testing.T) {
	outbound := newVintedStrictTestOutbound()
	for _, domain := range []string{"metrics-ingress-web.svc.vinted.com", "api-js.datadome.co", "ct.captcha-delivery.com"} {
		ctx := adapter.WithContext(context.Background(), &adapter.InboundContext{Domain: domain})
		if _, err := outbound.domainDestination(ctx, M.SocksaddrFrom(netip.MustParseAddr("198.51.100.10"), 443)); err == nil {
			t.Fatalf("IPv4 session domain %q was accepted by the IPv6 strict outbound", domain)
		}
	}
}

func TestVintedStrictOutboundUsesIPv6OnlyDNS(t *testing.T) {
	dnsRouter := &relayFallbackTestDNSRouter{addresses: []netip.Addr{
		netip.MustParseAddr("198.51.100.20"),
		netip.MustParseAddr("2001:db8::20"),
	}}
	outbound := newVintedStrictTestOutbound()
	outbound.dnsRouter = dnsRouter
	addresses, err := outbound.ipv6Addresses(context.Background(), "api.www.vinted.com")
	if err != nil {
		t.Fatal(err)
	}
	if dnsRouter.strategy != C.DomainStrategyIPv6Only {
		t.Fatalf("DNS strategy = %v, want IPv6 only", dnsRouter.strategy)
	}
	if len(addresses) != 1 || addresses[0].String() != "2001:db8::20" {
		t.Fatalf("IPv6 addresses = %v", addresses)
	}
}

func TestVintedStrictOutboundReturnsIPv6PacketDestination(t *testing.T) {
	dnsRouter := &relayFallbackTestDNSRouter{addresses: []netip.Addr{
		netip.MustParseAddr("198.51.100.20"),
		netip.MustParseAddr("2001:db8::20"),
	}}
	strictDialer := &vintedStrictTestDialer{}
	outbound := newVintedStrictTestOutbound()
	outbound.dnsRouter = dnsRouter
	outbound.dialer = strictDialer
	ctx := adapter.WithContext(context.Background(), &adapter.InboundContext{Domain: "api.www.vinted.com"})
	conn, address, err := outbound.ListenPacketWithDestination(ctx, M.SocksaddrFrom(netip.MustParseAddr("198.51.100.10"), 443))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if address.String() != "2001:db8::20" || strictDialer.destination.Addr != address {
		t.Fatalf("packet destination = %v / %v", address, strictDialer.destination)
	}
}
