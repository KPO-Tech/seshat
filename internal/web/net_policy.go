package web

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// HostResolver abstracts hostname resolution so network policy can be tested and
// reused by HTTP fetch, crawl, and future browser transports.
type HostResolver interface {
	LookupNetIP(ctx context.Context, network string, host string) ([]netip.Addr, error)
}

// DefaultResolver is the process-wide DNS resolver used when no explicit resolver
// is supplied by a web subsystem.
func DefaultResolver() HostResolver {
	return net.DefaultResolver
}

// RejectLocalNetworkTarget blocks obvious SSRF-style targets before any network work starts.
// This fast path still checks only the literal host so lightweight normalization
// can reject obviously local targets before any DNS work starts.
func RejectLocalNetworkTarget(parsed *url.URL) error {
	if parsed == nil {
		return fmt.Errorf("missing URL")
	}
	host := NormalizeHost(parsed.Hostname())
	return rejectHostAndIP(host)
}

// ResolveAndRejectLocalNetworkTarget hardens SSRF protection by resolving the
// hostname and rejecting any target that resolves to loopback, private, or local
// network addresses.
func ResolveAndRejectLocalNetworkTarget(ctx context.Context, parsed *url.URL, resolver HostResolver) error {
	if parsed == nil {
		return fmt.Errorf("missing URL")
	}
	host := NormalizeHost(parsed.Hostname())
	if err := rejectHostAndIP(host); err != nil {
		return err
	}
	if host == "" {
		return fmt.Errorf("missing host")
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return nil
	}
	if resolver == nil {
		resolver = DefaultResolver()
	}
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("resolve host %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("resolve host %q: no addresses returned", host)
	}
	for _, addr := range addrs {
		if err := rejectResolvedAddr(addr); err != nil {
			return err
		}
	}
	return nil
}

// RejectLocalDialTarget validates a host:port dial target before a transport
// opens a socket. This closes the gap where a public hostname resolves to a
// private address after initial URL validation.
func RejectLocalDialTarget(ctx context.Context, address string, resolver HostResolver) error {
	host, _, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		host = strings.TrimSpace(address)
	}
	host = NormalizeHost(host)
	if err := rejectHostAndIP(host); err != nil {
		return err
	}
	if host == "" {
		return fmt.Errorf("missing host")
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return nil
	}
	if resolver == nil {
		resolver = DefaultResolver()
	}
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("resolve dial host %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("resolve dial host %q: no addresses returned", host)
	}
	for _, addr := range addrs {
		if err := rejectResolvedAddr(addr); err != nil {
			return err
		}
	}
	return nil
}

func rejectHostAndIP(host string) error {
	switch host {
	case "", "localhost", "localhost.localdomain", "host.docker.internal":
		return fmt.Errorf("local network targets are not allowed")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return rejectResolvedAddr(ip)
	}
	if strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("local network targets are not allowed")
	}
	return nil
}

func rejectResolvedAddr(ip netip.Addr) error {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("private or loopback IP targets are not allowed")
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("unspecified IP targets are not allowed")
	}
	return nil
}
