package api

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"
)

// dnsResolver is an interface for DNS lookups, allowing test injection.
type dnsResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// defaultResolver wraps net.Resolver for production use.
type defaultResolver struct{}

func (defaultResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

// privateIPNets are the IP ranges that should be blocked for SSRF protection.
var privateIPNets = []net.IPNet{
	// IPv4 private ranges
	{IP: net.IP{127, 0, 0, 0}, Mask: net.CIDRMask(8, 32)},       // 127.0.0.0/8 loopback
	{IP: net.IP{10, 0, 0, 0}, Mask: net.CIDRMask(8, 32)},        // 10.0.0.0/8
	{IP: net.IP{172, 16, 0, 0}, Mask: net.CIDRMask(12, 32)},     // 172.16.0.0/12
	{IP: net.IP{192, 168, 0, 0}, Mask: net.CIDRMask(16, 32)},    // 192.168.0.0/16
	{IP: net.IP{169, 254, 0, 0}, Mask: net.CIDRMask(16, 32)},    // 169.254.0.0/16 link-local
	{IP: net.IP{0, 0, 0, 0}, Mask: net.CIDRMask(32, 32)},         // 0.0.0.0
	{IP: net.IP{255, 255, 255, 255}, Mask: net.CIDRMask(32, 32)}, // 255.255.255.255 broadcast
	{IP: net.IP{224, 0, 0, 0}, Mask: net.CIDRMask(4, 32)},        // 224.0.0.0/4 IPv4 multicast
	// IPv6 private ranges
	{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)},       // ::1 loopback
	{IP: net.ParseIP("fc00::"), Mask: net.CIDRMask(7, 128)},      // fc00::/7 unique local
	{IP: net.ParseIP("fe80::"), Mask: net.CIDRMask(10, 128)},     // fe80::/10 link-local
	{IP: net.ParseIP("ff00::"), Mask: net.CIDRMask(8, 128)},      // ff00::/8 IPv6 multicast
}

// isPrivateIP reports whether ip is in a private/internal network range.
// It also checks the IPv4 form of IPv4-mapped IPv6 addresses (e.g. ::ffff:127.0.0.1).
func isPrivateIP(ip net.IP) bool {
	for _, n := range privateIPNets {
		if n.Contains(ip) {
			return true
		}
	}
	// Check IPv4-mapped IPv6 addresses (e.g. ::ffff:10.0.0.1).
	if v4 := ip.To4(); v4 != nil && !v4.Equal(ip) {
		for _, n := range privateIPNets {
			if n.Contains(v4) {
				return true
			}
		}
	}
	return false
}

// validateProbeURL validates a URL for SSRF safety. It checks scheme,
// parses the host, and resolves DNS to ensure the target is not a private IP.
func validateProbeURL(ctx context.Context, rawURL string, resolver dnsResolver) error {
	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL scheme %q is not allowed; use http or https", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL must contain a host")
	}

	// Check if host is a literal IP address.
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("URL host %q resolves to a private/internal IP address", host)
		}
		return nil
	}

	// Resolve hostname to IPs and check each one.
	dnsCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	addrs, err := resolver.LookupIPAddr(dnsCtx, host)
	if err != nil {
		return fmt.Errorf("failed to resolve host %q: %w", host, err)
	}

	for _, addr := range addrs {
		if isPrivateIP(addr.IP) {
			return fmt.Errorf("URL host %q resolves to a private/internal IP address", host)
		}
	}

	return nil
}
