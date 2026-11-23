// Package indexerprobe tests indexers from the catalog that have not been
// added, so the add-indexer page can recommend ones that work. It also
// holds the release-matching and error-classification helpers shared with
// the torznab proxy's search stats.
package indexerprobe

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/beacon-stack/pulse/internal/core/indexer"
	"github.com/beacon-stack/pulse/internal/scraper"
	"github.com/beacon-stack/pulse/internal/sitenet"
)

// SearchMode maps a torznab t= function to "movie", "tv" or "search".
func SearchMode(t string) string {
	switch t {
	case "movie", "movie-search":
		return "movie"
	case "tvsearch", "tv-search":
		return "tv"
	default:
		return "search"
	}
}

var btihRe = regexp.MustCompile(`(?i)urn:btih:([a-z0-9]+)`)

// ReleaseIDOf identifies a release across indexers: its info hash when known
// (directly or from a magnet link), plus its normalized title and size so
// indexers that don't expose hashes can still be matched.
func ReleaseIDOf(res scraper.SearchResult) indexer.ReleaseID {
	hash := strings.ToLower(res.InfoHash)
	if hash == "" {
		for _, link := range []string{res.MagnetURI, res.Download} {
			if m := btihRe.FindStringSubmatch(link); m != nil {
				hash = strings.ToLower(m[1])
				break
			}
		}
	}
	return indexer.ReleaseID{Hash: hash, Title: normalizeTitle(res.Title), Size: res.Size}
}

// normalizeTitle lower-cases a release title and reduces it to words, so
// "Big.Buck.Bunny.2008" and "Big Buck Bunny (2008)" compare equal.
func normalizeTitle(title string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}), " ")
}

// ErrorKind classifies a search error for the stats ("" on success).
func ErrorKind(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case indexer.IsLikelyIPBan(msg):
		return "ip_banned"
	case scraper.IsCloudflareError(err):
		return "cloudflare_no_solver"
	case strings.Contains(msg, "flaresolverr"):
		return "cloudflare_solve_failed"
	case sitenet.IsProxyError(err):
		return "proxy_down"
	case strings.Contains(msg, "http request failed"):
		return "unreachable"
	case strings.Contains(msg, "http 429"):
		return "rate_limited"
	case strings.HasPrefix(msg, "http ") || strings.Contains(msg, ": http "):
		return "http_error"
	default:
		return "other"
	}
}
