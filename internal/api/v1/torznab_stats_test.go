package v1

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/beacon-stack/pulse/internal/scraper"
)

func TestReleaseKey(t *testing.T) {
	hash := "ABCDEF0123456789ABCDEF0123456789ABCDEF01"
	cases := []struct {
		name string
		res  scraper.SearchResult
		want string
	}{
		{"info hash", scraper.SearchResult{InfoHash: hash}, "h:abcdef0123456789abcdef0123456789abcdef01"},
		{"magnet", scraper.SearchResult{MagnetURI: "magnet:?xt=urn:btih:" + hash + "&dn=x"}, "h:abcdef0123456789abcdef0123456789abcdef01"},
		{"title fallback", scraper.SearchResult{Title: "Dune.Part.Two (2024) 1080p", Size: 42}, "t:dune part two 2024 1080p|42"},
	}
	for _, c := range cases {
		if got := releaseKey(c.res); got != c.want {
			t.Errorf("%s: releaseKey = %q, want %q", c.name, got, c.want)
		}
	}
	// The same release listed by two indexers with different punctuation matches.
	a := releaseKey(scraper.SearchResult{Title: "Dune.Part.Two.2024.1080p", Size: 7})
	b := releaseKey(scraper.SearchResult{Title: "Dune Part Two 2024 1080p", Size: 7})
	if a != b {
		t.Errorf("title fallback keys differ: %q vs %q", a, b)
	}
}

func TestSearchErrorKind(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{fmt.Errorf("HTTP request failed: %w", context.DeadlineExceeded), "timeout"},
		{&scraper.CloudflareError{StatusCode: 403}, "cloudflare_no_solver"},
		{errors.New("FlareSolverr failed: timeout after 60s"), "cloudflare_solve_failed"},
		{errors.New("Cloudflare has blocked this request (code 1020)"), "ip_banned"},
		{errors.New("HTTP request failed: dial tcp: no such host"), "unreachable"},
		{errors.New("HTTP 502"), "http_error"},
		{errors.New("parsing HTML: bad selector"), "other"},
	}
	for _, c := range cases {
		if got := searchErrorKind(c.err); got != c.want {
			t.Errorf("searchErrorKind(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}
