package sitenet

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestIsLocalHost(t *testing.T) {
	local := []string{"jackett", "prowlarr", "localhost", "127.0.0.1", "192.168.1.20", "10.0.0.5", "172.29.0.4", "::1", "nas.local", "box.lan", "fe80::1"}
	remote := []string{"1337x.to", "thepiratebay.org", "8.8.8.8", "nyaa.si", ""}
	for _, h := range local {
		if !IsLocalHost(h) {
			t.Errorf("%q should be local", h)
		}
	}
	for _, h := range remote {
		if IsLocalHost(h) {
			t.Errorf("%q should not be local", h)
		}
	}
}

func TestProxyFor(t *testing.T) {
	t.Cleanup(func() { _ = SetProxy("") })
	if err := SetProxy("http://vpn:8888"); err != nil {
		t.Fatal(err)
	}
	req := func(raw string) *http.Request {
		u, _ := url.Parse(raw)
		return &http.Request{URL: u}
	}
	if p, _ := proxyFor(req("https://1337x.to/search/x/1/")); p == nil || p.Host != "vpn:8888" {
		t.Errorf("site request proxy = %v, want vpn:8888", p)
	}
	if p, _ := proxyFor(req("http://jackett:9117/api")); p != nil {
		t.Errorf("local request proxy = %v, want direct", p)
	}
	for _, bad := range []string{"vpn:8888", "ftp://x:1", "http://"} {
		if err := SetProxy(bad); err == nil {
			t.Errorf("SetProxy(%q) accepted an invalid URL", bad)
		}
	}
}

// A request through a dead proxy must fail, not fall back to direct.
func TestNoFallbackWhenProxyDown(t *testing.T) {
	t.Cleanup(func() { _ = SetProxy("") })
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("reached directly"))
	}))
	defer site.Close()
	// The test server is on 127.0.0.1, which is local and would bypass the
	// proxy, so address it by a public-looking name that resolves to it.
	addr := strings.TrimPrefix(site.URL, "http://127.0.0.1")
	if err := SetProxy("http://127.0.0.1:1"); err != nil { // nothing listens on port 1
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com"+addr, nil)
	resp, err := Client(0).Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("request succeeded with the proxy down; it must fail instead")
	}
	if !IsProxyError(err) {
		t.Errorf("error %v not recognised as a proxy error", err)
	}
}
