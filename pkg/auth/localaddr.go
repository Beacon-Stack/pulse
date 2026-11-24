package auth

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// isLocalAddr reports whether ip is on the local machine or network:
// loopback, the private ranges (RFC 1918, RFC 4193), link-local, or one
// of the extra ranges the owner listed.
func isLocalAddr(ip netip.Addr, extra []netip.Prefix) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	for _, p := range extra {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func allLocal(addrs []netip.Addr, extra []netip.Prefix) bool {
	for _, ip := range addrs {
		if !isLocalAddr(ip, extra) {
			return false
		}
	}
	return len(addrs) > 0
}

func parseIP(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"`)
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	if i := strings.IndexByte(s, '%'); i >= 0 { // IPv6 zone
		s = s[:i]
	}
	ip, err := netip.ParseAddr(s)
	return ip, err == nil
}

// clientAddrs returns the address that connected, followed by every
// client address a proxy reported, in the order given. ok is false when
// any of them can't be parsed.
func clientAddrs(r *http.Request) (addrs []netip.Addr, ok bool) {
	peer, ok := parseIP(r.RemoteAddr)
	if !ok {
		return nil, false
	}
	addrs = append(addrs, peer)

	var forwarded []string
	for _, h := range []string{"X-Real-Ip", "Cf-Connecting-Ip", "True-Client-Ip"} {
		forwarded = append(forwarded, r.Header.Values(h)...)
	}
	for _, v := range r.Header.Values("Forwarded") {
		for _, elem := range strings.Split(v, ",") {
			for _, pair := range strings.Split(elem, ";") {
				k, val, found := strings.Cut(strings.TrimSpace(pair), "=")
				if found && strings.EqualFold(k, "for") {
					forwarded = append(forwarded, val)
				}
			}
		}
	}
	// X-Forwarded-For lists the original client first; keep it last here
	// so the last address is the best guess at who is really asking.
	var xff []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		xff = append(xff, strings.Split(v, ",")...)
	}
	for i := len(xff) - 1; i >= 0; i-- {
		forwarded = append(forwarded, xff[i])
	}

	ok = true
	for _, f := range forwarded {
		if strings.TrimSpace(f) == "" {
			continue
		}
		ip, parsed := parseIP(f)
		if !parsed {
			ok = false
			continue
		}
		addrs = append(addrs, ip)
	}
	return addrs, ok
}

// clientIsLocal reports whether the request came from the local network.
//
// The address that connected must be local. When it is, it may be a
// reverse proxy relaying a request from the internet, so every client
// address the proxy reports must be local too. An address that can't be
// parsed counts as not local. A client can only use these headers to
// make itself look remote, never local, because a remote peer already
// fails the first check.
func clientIsLocal(r *http.Request, extra []netip.Prefix) bool {
	addrs, ok := clientAddrs(r)
	return ok && allLocal(addrs, extra)
}

// parseNetwork reads one address range: CIDR ("100.64.0.0/10") or a
// single address.
func parseNetwork(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Prefix{}, fmt.Errorf("empty network")
	}
	if p, err := netip.ParsePrefix(s); err == nil {
		return p.Masked(), nil
	}
	if ip, err := netip.ParseAddr(s); err == nil {
		return netip.PrefixFrom(ip, ip.BitLen()), nil
	}
	return netip.Prefix{}, fmt.Errorf("%q is not an address range like 192.168.1.0/24", s)
}

// ParseNetworks reads a list of address ranges, skipping blanks and
// duplicates. It fails on the first entry that isn't one, and on a range
// so wide it would make the whole internet local.
func ParseNetworks(list []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	seen := map[netip.Prefix]bool{}
	for _, s := range list {
		if strings.TrimSpace(s) == "" {
			continue
		}
		p, err := parseNetwork(s)
		if err != nil {
			return nil, err
		}
		if p.Bits() < 8 {
			return nil, fmt.Errorf("%q covers too much of the internet to count as a local network", s)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, nil
}

func networkStrings(nets []netip.Prefix) []string {
	out := make([]string, 0, len(nets))
	for _, p := range nets {
		out = append(out, p.String())
	}
	return out
}
