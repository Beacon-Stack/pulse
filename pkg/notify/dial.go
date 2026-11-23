package notify

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Outbound connections are checked before they're made, so a channel's
// settings can't be used to reach things they shouldn't.
//
//   - Internet-only services (Discord, Slack, Telegram, Pushover) may not
//     reach private, loopback or link-local addresses at all.
//   - Services people run themselves (ntfy, Gotify, webhooks, an SMTP
//     relay) may reach the home network, but never the cloud-metadata
//     ranges.

func mustCIDRs(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic("notify: bad CIDR " + c)
		}
		out = append(out, n)
	}
	return out
}

var (
	internetOnlyBlocked = mustCIDRs(
		"127.0.0.0/8", "::1/128", // loopback
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7", // private
		"169.254.0.0/16", "fe80::/10", // link-local, incl. cloud metadata
		"0.0.0.0/8", "100.64.0.0/10", // "this" network, CGNAT
	)
	homeNetworkBlocked = mustCIDRs(
		"169.254.0.0/16", "fe80::/10", // cloud metadata lives here
	)
)

func blocked(ip net.IP, ranges []*net.IPNet) bool {
	for _, n := range ranges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func dialer(ranges []*net.IPNet) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("bad address %q: %w", addr, err)
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("can't find %s: %w", host, err)
		}
		for _, ip := range ips {
			if blocked(ip, ranges) {
				return nil, fmt.Errorf("connecting to %s (%s) is not allowed", host, ip)
			}
		}
		// Dial the address we checked, so DNS can't change it in between.
		d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
}

func transport(ranges []*net.IPNet) *http.Transport {
	return &http.Transport{
		DialContext:           dialer(ranges),
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       60 * time.Second,
	}
}

// These are variables so tests can point providers at a local server.
var (
	internetClient = &http.Client{Timeout: SendTimeout, Transport: transport(internetOnlyBlocked)}
	homeClient     = &http.Client{Timeout: SendTimeout, Transport: transport(homeNetworkBlocked)}
	homeDial       = dialer(homeNetworkBlocked)
)
