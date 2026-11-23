// Package sitenet is the network path Pulse uses to reach indexer sites:
// searches, .torrent downloads and connection tests. When a proxy is set
// (e.g. a VPN container's HTTP proxy), site traffic goes through it and
// never falls back to a direct connection if the proxy is down. Local
// addresses (Docker service names, LAN and loopback IPs) always connect
// directly, so self-hosted Torznab endpoints keep working.
package sitenet

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

var proxyURL atomic.Pointer[url.URL]

// SetProxy routes site traffic through raw (http://host:port). Empty clears
// it, restoring direct connections (and the standard proxy env vars).
func SetProxy(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		proxyURL.Store(nil)
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") {
		return fmt.Errorf("invalid scraper proxy URL %q: want http://host:port", raw)
	}
	proxyURL.Store(u)
	return nil
}

// Proxy reports the configured proxy, or nil.
func Proxy() *url.URL {
	return proxyURL.Load()
}

// proxyFor picks the proxy for one request.
func proxyFor(req *http.Request) (*url.URL, error) {
	p := proxyURL.Load()
	if p == nil {
		return http.ProxyFromEnvironment(req)
	}
	if IsLocalHost(req.URL.Hostname()) {
		return nil, nil
	}
	return p, nil
}

// IsLocalHost reports whether host is on this machine or the local network:
// a single-label name (a Docker service such as "jackett"), *.local /
// *.lan / *.internal / localhost, or a loopback, private or link-local IP.
func IsLocalHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
	}
	if !strings.Contains(host, ".") {
		return true
	}
	for _, suffix := range []string{".local", ".lan", ".internal", ".localhost", ".home.arpa"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// transport is shared so connections to the proxy are reused.
var transport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = proxyFor
	t.TLSHandshakeTimeout = 15 * time.Second
	return t
}()

// Transport returns the round tripper for requests to indexer sites.
func Transport() http.RoundTripper {
	return transport
}

// Client returns an HTTP client for indexer sites with the given timeout
// (0 means none; rely on the request context).
func Client(timeout time.Duration) *http.Client {
	return &http.Client{Transport: transport, Timeout: timeout}
}

// IsProxyError reports whether err came from failing to reach the proxy.
func IsProxyError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "proxyconnect")
}
