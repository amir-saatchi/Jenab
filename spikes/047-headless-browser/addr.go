package main

// Address rules copied from internal/web/fetch.go (SPEC 6.7, TASK-036).

import "net/netip"

var (
	shared   = netip.MustParsePrefix("100.64.0.0/10") // carrier-grade NAT
	thisNet  = netip.MustParsePrefix("0.0.0.0/8")
	nat64    = netip.MustParsePrefix("64:ff9b::/96")
	nat64Own = netip.MustParsePrefix("64:ff9b:1::/48") // local-use NAT64
	sixTo4   = netip.MustParsePrefix("2002::/16")
	compat   = netip.MustParsePrefix("::/96") // IPv4-compatible
)

func onThisComputer(a netip.Addr) bool {
	a = a.Unmap()
	if v4 := carried(a); v4.IsValid() && onThisComputer(v4) {
		return true
	}
	return a.IsLoopback() || a.IsUnspecified() || thisNet.Contains(a)
}
func carried(a netip.Addr) netip.Addr {
	if !a.Is6() {
		return netip.Addr{}
	}
	b := a.As16()
	switch {
	case nat64.Contains(a), compat.Contains(a) && !a.IsLoopback() && !a.IsUnspecified():
		return netip.AddrFrom4([4]byte(b[12:16]))
	case sixTo4.Contains(a):
		return netip.AddrFrom4([4]byte(b[2:6]))
	}
	return netip.Addr{}
}
func blocked(a netip.Addr) string {
	a = a.Unmap()
	if v4 := carried(a); v4.IsValid() {
		if why := blocked(v4); why != "" {
			return why + " inside an IPv6 address"
		}
	}
	switch {
	case !a.IsValid():
		return "not an address"
	case a.IsLoopback():
		return "a loopback address"
	case a.IsPrivate():
		return "a private address"
	case a.IsLinkLocalUnicast(), a.IsLinkLocalMulticast():
		return "a link-local address"
	case a.IsUnspecified():
		return "an unspecified address"
	case a.IsMulticast(), a.IsInterfaceLocalMulticast():
		return "a multicast address"
	case shared.Contains(a):
		return "a shared (carrier-grade NAT) address"
	case thisNet.Contains(a), nat64Own.Contains(a):
		return "a reserved address"
	}
	return ""
}
