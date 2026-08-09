package ai

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
)

var blockedProviderPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func ValidateDialIP(ip net.IP) error {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return errors.New("provider DNS returned an invalid address")
	}
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() || address.IsUnspecified() {
		return errors.New("provider DNS returned a non-public address")
	}
	for _, prefix := range blockedProviderPrefixes {
		if prefix.Contains(address) {
			return errors.New("provider DNS returned a reserved address")
		}
	}
	return nil
}

type safeDialer struct {
	resolver *net.Resolver
	dialer   *net.Dialer
}

func newSafeDialer() *safeDialer {
	return &safeDialer{resolver: net.DefaultResolver, dialer: &net.Dialer{}}
}

func (dialer *safeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return nil, errors.New("provider network address is invalid")
	}
	resolved, err := dialer.resolver.LookupIPAddr(ctx, host)
	if err != nil || len(resolved) == 0 {
		return nil, errors.New("provider DNS lookup failed")
	}
	for _, candidate := range resolved {
		if err := ValidateDialIP(candidate.IP); err != nil {
			return nil, err
		}
	}
	var lastErr error
	for _, candidate := range resolved {
		target := net.JoinHostPort(candidate.IP.String(), port)
		connection, err := dialer.dialer.DialContext(ctx, network, target)
		if err == nil {
			return connection, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, errors.New("provider connection failed")
	}
	return nil, errors.New("provider connection failed")
}
