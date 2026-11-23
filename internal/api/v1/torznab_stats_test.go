package v1

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/beacon-stack/pulse/internal/scraper"
)

func TestReleaseID(t *testing.T) {
	hash := "ABCDEF0123456789ABCDEF0123456789ABCDEF01"
	want := "abcdef0123456789abcdef0123456789abcdef01"
	cases := []struct {
		name string
		res  scraper.SearchResult
		hash string
	}{
		{"info hash", scraper.SearchResult{InfoHash: hash}, want},
		{"magnet", scraper.SearchResult{MagnetURI: "magnet:?xt=urn:btih:" + hash + "&dn=x"}, want},
		{"no hash", scraper.SearchResult{Download: "https://example.org/t/1.torrent"}, ""},
	}
	for _, c := range cases {
		if got := releaseID(c.res).Hash; got != c.hash {
			t.Errorf("%s: hash = %q, want %q", c.name, got, c.hash)
		}
	}
	id := releaseID(scraper.SearchResult{Title: "Dune.Part.Two (2024) 1080p", Size: 42})
	if id.Title != "dune part two 2024 1080p" || id.Size != 42 {
		t.Errorf("title/size = %q/%d", id.Title, id.Size)
	}
	// The same release listed with different punctuation normalizes alike.
	if normalizeTitle("Big.Buck.Bunny.2008.720p") != normalizeTitle("Big Buck Bunny (2008) 720p") {
		t.Error("titles with different punctuation should normalize equal")
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
