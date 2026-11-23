package indexerprobe

import (
	"context"
	"math"
	"sort"
	"time"
)

// Verdicts, best first.
const (
	VerdictWorks             = "works"              // last test found results
	VerdictNoResults         = "no_results"         // answers, but never finds anything
	VerdictFlaky             = "flaky"              // last test failed, earlier ones didn't
	VerdictNeedsFlareSolverr = "needs_flaresolverr" // Cloudflare and no FlareSolverr set up
	VerdictBlocked           = "blocked"            // the site has blocked this IP
	VerdictDown              = "down"               // failed 3+ tests in a row
)

// SiteHealth summarizes a catalog indexer's tests over the last 30 days.
type SiteHealth struct {
	CatalogID string `json:"catalog_id"`
	Verdict   string `json:"verdict"`
	// Score ranks sites for "Recommended" (0–100, higher is better).
	Score       int     `json:"score"`
	Tests       int     `json:"tests"`
	Failures    int     `json:"failures"`
	SuccessRate float64 `json:"success_rate"`
	// Over successful tests:
	MedianMs      int     `json:"median_ms"`
	AvgResults    float64 `json:"avg_results"`
	MedianSeeders int     `json:"median_seeders"`
	// Comparisons counts tests whose results were compared with other
	// sites'; UniqueShare is the share of results nobody else found (0–1).
	Comparisons      int     `json:"comparisons"`
	UniqueShare      float64 `json:"unique_share"`
	BehindCloudflare float64 `json:"behind_cloudflare"`
	SolveRate        float64 `json:"solve_rate"`

	ConsecutiveFailures int    `json:"consecutive_failures"`
	LastTestAt          string `json:"last_test_at"`
	LastOKAt            string `json:"last_ok_at,omitempty"`
	LastErrorKind       string `json:"last_error_kind,omitempty"`
	LastError           string `json:"last_error,omitempty"`
}

// Health summarizes every tested catalog indexer, keyed by catalog ID.
func (p *Prober) Health(ctx context.Context) (map[string]*SiteHealth, error) {
	since := p.now().Add(-retention).UTC().Format(time.RFC3339)
	rows, err := p.q.ListCatalogProbesSince(ctx, since)
	if err != nil {
		return nil, err
	}
	type acc struct {
		durations, seeders           []int
		results, challenged, solved  int
		compResults, compUnique, oks int
	}
	accs := make(map[string]*acc)
	out := make(map[string]*SiteHealth)
	for _, r := range rows { // ordered by catalog_id, at
		h := out[r.CatalogID]
		a := accs[r.CatalogID]
		if h == nil {
			h = &SiteHealth{CatalogID: r.CatalogID}
			a = &acc{}
			out[r.CatalogID], accs[r.CatalogID] = h, a
		}
		h.Tests++
		h.LastTestAt = r.At
		if r.Challenged {
			a.challenged++
			if r.Solved {
				a.solved++
			}
		}
		if !r.Ok {
			h.Failures++
			h.ConsecutiveFailures++
			h.LastErrorKind, h.LastError = r.ErrorKind, r.ErrorMessage
			continue
		}
		h.ConsecutiveFailures, h.LastErrorKind, h.LastError = 0, "", ""
		h.LastOKAt = r.At
		a.oks++
		a.durations = append(a.durations, int(r.DurationMs))
		a.results += int(r.Results)
		if r.Results > 0 {
			a.seeders = append(a.seeders, int(r.MedianSeeders))
		}
		if r.UniqueResults >= 0 {
			h.Comparisons++
			a.compResults += int(r.Results)
			a.compUnique += int(r.UniqueResults)
		}
	}
	for id, h := range out {
		a := accs[id]
		h.SuccessRate = ratio(a.oks, h.Tests)
		h.MedianMs = median(a.durations)
		h.AvgResults = ratio(a.results, a.oks)
		h.MedianSeeders = median(a.seeders)
		h.UniqueShare = ratio(a.compUnique, a.compResults)
		h.BehindCloudflare = ratio(a.challenged, h.Tests)
		h.SolveRate = ratio(a.solved, a.challenged)
		h.Verdict = verdict(h)
		h.Score = score(h)
	}
	return out, nil
}

func verdict(h *SiteHealth) string {
	if h.ConsecutiveFailures == 0 {
		if h.AvgResults == 0 {
			return VerdictNoResults
		}
		return VerdictWorks
	}
	switch {
	case h.LastErrorKind == "cloudflare_no_solver":
		return VerdictNeedsFlareSolverr
	case h.LastErrorKind == "ip_banned":
		return VerdictBlocked
	case h.ConsecutiveFailures >= failingAfter || h.LastOKAt == "":
		return VerdictDown
	default:
		return VerdictFlaky
	}
}

// score ranks working sites by seeders (up to 35), results only they find
// (up to 20) and speed (up to 15) on top of a base for working at all,
// scaled by reliability, with a small cost for needing FlareSolverr.
func score(h *SiteHealth) int {
	switch h.Verdict {
	case VerdictWorks:
	case VerdictFlaky:
		return int(10 * h.SuccessRate)
	case VerdictNoResults:
		return 3
	default:
		return 0
	}
	s := 30.0
	s += math.Min(35, 12*math.Log10(1+float64(h.MedianSeeders)))
	if h.Comparisons > 0 {
		s += 20 * h.UniqueShare
	} else {
		s += 5 // unknown: assume a little
	}
	switch {
	case h.MedianMs <= 3000:
		s += 15
	case h.MedianMs < 60000:
		s += 15 * float64(60000-h.MedianMs) / 57000
	}
	s *= 0.5 + 0.5*h.SuccessRate
	if h.BehindCloudflare >= 0.5 {
		s -= 5
	}
	return int(math.Round(math.Max(1, math.Min(100, s))))
}

func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

func median(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m]
	}
	return (s[m-1] + s[m]) / 2
}
