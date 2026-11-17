package core

import (
	"context"
	"net"
	"net/netip"
	"strings"

	"github.com/sagernet/sing-box/adapter"
	sbOutbound "github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/common/dialer"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

const VintedStrictOutboundType = "vinted-strict"

type VintedStrictOutboundOptions struct {
	option.DialerOptions
	AllowedDomains        []string `json:"allowed_domains"`
	AllowedDomainSuffixes []string `json:"allowed_domain_suffixes"`
	IPv4Domains           []string `json:"ipv4_domains,omitempty"`
	IPv4DomainSuffixes    []string `json:"ipv4_domain_suffixes,omitempty"`
}

type vintedStrictOutbound struct {
	sbOutbound.Adapter
	dialer                N.Dialer
	dnsRouter             adapter.DNSRouter
	allowedDomains        map[string]bool
	allowedDomainSuffixes []string
	ipv4Domains           map[string]bool
	ipv4DomainSuffixes    []string
}

var _ adapter.Outbound = (*vintedStrictOutbound)(nil)

func registerVintedStrictOutbound(registry *sbOutbound.Registry) {
	sbOutbound.Register[VintedStrictOutboundOptions](registry, VintedStrictOutboundType, newVintedStrictOutbound)
}

func newVintedStrictOutbound(ctx context.Context, _ adapter.Router, _ log.ContextLogger, tag string, options VintedStrictOutboundOptions) (adapter.Outbound, error) {
	if len(options.AllowedDomains) == 0 && len(options.AllowedDomainSuffixes) == 0 {
		return nil, E.New("Vinted strict outbound requires an allowed domain list")
	}
	dnsRouter := service.FromContext[adapter.DNSRouter](ctx)
	if dnsRouter == nil {
		return nil, E.New("Vinted strict outbound requires a DNS router")
	}
	options.UDPFragmentDefault = true
	strictDialer, err := dialer.NewWithOptions(dialer.Options{
		Context:        ctx,
		Options:        options.DialerOptions,
		RemoteIsDomain: false,
		DirectOutbound: true,
	})
	if err != nil {
		return nil, err
	}
	return &vintedStrictOutbound{
		Adapter:               sbOutbound.NewAdapterWithDialerOptions(VintedStrictOutboundType, tag, []string{N.NetworkTCP, N.NetworkUDP}, options.DialerOptions),
		dialer:                strictDialer,
		dnsRouter:             dnsRouter,
		allowedDomains:        normalizedDomainSet(options.AllowedDomains),
		allowedDomainSuffixes: normalizedDomains(options.AllowedDomainSuffixes),
		ipv4Domains:           normalizedDomainSet(options.IPv4Domains),
		ipv4DomainSuffixes:    normalizedDomains(options.IPv4DomainSuffixes),
	}, nil
}

func (o *vintedStrictOutbound) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	domainDestination, err := o.domainDestination(ctx, destination)
	if err != nil {
		return nil, err
	}
	addresses, err := o.ipv6Addresses(ctx, domainDestination.Fqdn)
	if err != nil {
		return nil, err
	}
	var dialErrors []error
	for _, address := range addresses {
		conn, dialErr := o.dialer.DialContext(ctx, network, M.SocksaddrFrom(address, domainDestination.Port))
		if dialErr == nil {
			return conn, nil
		}
		dialErrors = append(dialErrors, dialErr)
	}
	return nil, E.Cause(E.Errors(dialErrors...), "all Vinted IPv6 addresses failed")
}

func (o *vintedStrictOutbound) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	conn, _, err := o.ListenPacketWithDestination(ctx, destination)
	return conn, err
}

func (o *vintedStrictOutbound) ListenPacketWithDestination(ctx context.Context, destination M.Socksaddr) (net.PacketConn, netip.Addr, error) {
	domainDestination, err := o.domainDestination(ctx, destination)
	if err != nil {
		return nil, netip.Addr{}, err
	}
	addresses, err := o.ipv6Addresses(ctx, domainDestination.Fqdn)
	if err != nil {
		return nil, netip.Addr{}, err
	}
	var listenErrors []error
	for _, address := range addresses {
		conn, listenErr := o.dialer.ListenPacket(ctx, M.SocksaddrFrom(address, domainDestination.Port))
		if listenErr == nil {
			return conn, address, nil
		}
		listenErrors = append(listenErrors, listenErr)
	}
	return nil, netip.Addr{}, E.Cause(E.Errors(listenErrors...), "all Vinted IPv6 packet destinations failed")
}

func (o *vintedStrictOutbound) domainDestination(ctx context.Context, destination M.Socksaddr) (M.Socksaddr, error) {
	domain := ""
	if destination.IsDomain() {
		domain = destination.Fqdn
	} else if metadata := adapter.ContextFrom(ctx); metadata != nil {
		domain = metadata.Domain
	}
	domain = normalizeDomain(domain)
	if domain == "" {
		return M.Socksaddr{}, E.New("Vinted strict outbound rejected an IP literal without a sniffed domain")
	}
	if domainMatches(domain, o.ipv4Domains, o.ipv4DomainSuffixes) {
		return M.Socksaddr{}, E.New("Vinted strict outbound rejected an IPv4-only session domain: ", domain)
	}
	if !domainMatches(domain, o.allowedDomains, o.allowedDomainSuffixes) {
		return M.Socksaddr{}, E.New("Vinted strict outbound rejected an unapproved domain: ", domain)
	}
	return M.Socksaddr{Fqdn: domain, Port: destination.Port}, nil
}

func (o *vintedStrictOutbound) ipv6Addresses(ctx context.Context, domain string) ([]netip.Addr, error) {
	addresses, err := o.dnsRouter.Lookup(ctx, domain, adapter.DNSQueryOptions{Strategy: C.DomainStrategyIPv6Only})
	if err != nil {
		return nil, E.Cause(err, "Vinted IPv6 DNS lookup failed")
	}
	addresses6 := make([]netip.Addr, 0, len(addresses))
	for _, address := range addresses {
		if address.Is6() && !address.Is4In6() {
			addresses6 = append(addresses6, address)
		}
	}
	if len(addresses6) == 0 {
		return nil, E.New("Vinted domain has no IPv6 address: ", domain)
	}
	return addresses6, nil
}

func normalizedDomainSet(domains []string) map[string]bool {
	set := make(map[string]bool, len(domains))
	for _, domain := range normalizedDomains(domains) {
		set[domain] = true
	}
	return set
}

func normalizedDomains(domains []string) []string {
	normalized := make([]string, 0, len(domains))
	for _, domain := range domains {
		if value := normalizeDomain(domain); value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}

func normalizeDomain(domain string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
}

func domainMatches(domain string, exact map[string]bool, suffixes []string) bool {
	if exact[domain] {
		return true
	}
	for _, suffix := range suffixes {
		if domain == suffix || strings.HasSuffix(domain, "."+suffix) {
			return true
		}
	}
	return false
}
