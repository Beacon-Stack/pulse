package indexer

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	db "github.com/beacon-stack/pulse/internal/db/generated"
)

// Search stats: what each indexer is actually like to search, measured
// from the real searches Prism and Pilot send through Pulse. Two signals:
//
//   - one event per real scrape (speed, results, seeders, Cloudflare,
//     error kind), recorded by RecordSearch;
//   - overlap: when several indexers answer the same query within
//     overlapWindow, how many of each one's results no other indexer
//     returned (NoteResults, finalized by FlushOverlap).
//
// Everything is kept for statsRetention and summarized by Summarize.

const (
	// statsRetention is how far back stats look and how long rows are kept.
	statsRetention = 30 * 24 * time.Hour
	// overlapWindow groups answers to the same query: Prism and Pilot fan a
	// search out to every indexer at once, and slow (Cloudflare) indexers
	// can take a couple of minutes to answer.
	overlapWindow = 5 * time.Minute
	// maxOverlapGroups bounds memory if searches arrive faster than the
	// window closes; the oldest groups are finalized early.
	maxOverlapGroups = 2000
	// seederSample is how many of the best-seeded results the per-search
	// median looks at: "how well seeded are the results you'd pick".
	seederSample = 20
)

// SearchEvent is one real scrape's outcome.
type SearchEvent struct {
	IndexerID string
	At        time.Time
	Duration  time.Duration
	// ErrorKind is empty on success, else a short class such as
	// "timeout", "unreachable", "http_error", "cloudflare_no_solver",
	// "cloudflare_solve_failed", "ip_banned" or "other".
	ErrorKind string
	// Seeders has one entry per result (0 when unknown).
	Seeders    []int
	Challenged bool
	Solved     bool
}

type overlapGroup struct {
	first     time.Time
	byIndexer map[string]map[string]struct{}
}

// Stats records and summarizes per-indexer search stats.
type Stats struct {
	q      db.Querier
	logger *slog.Logger
	now    func() time.Time

	mu     sync.Mutex
	groups map[string]*overlapGroup
}

// NewStats returns a recorder backed by q.
func NewStats(q db.Querier, logger *slog.Logger) *Stats {
	return &Stats{q: q, logger: logger, now: time.Now, groups: make(map[string]*overlapGroup)}
}

// RecordSearch stores one scrape's outcome.
func (s *Stats) RecordSearch(ctx context.Context, ev SearchEvent) {
	withSeeders := 0
	for _, n := range ev.Seeders {
		if n > 0 {
			withSeeders++
		}
	}
	err := s.q.InsertSearchEvent(ctx, db.InsertSearchEventParams{
		IndexerID:     ev.IndexerID,
		At:            ev.At.UTC().Format(time.RFC3339),
		DurationMs:    ev.Duration.Milliseconds(),
		Ok:            ev.ErrorKind == "",
		ErrorKind:     ev.ErrorKind,
		Results:       int64(len(ev.Seeders)),
		WithSeeders:   int64(withSeeders),
		MedianSeeders: int64(topMedian(ev.Seeders, seederSample)),
		Challenged:    ev.Challenged,
		Solved:        ev.Challenged && ev.Solved,
	})
	if err != nil {
		s.logger.Warn("indexer stats: recording search failed", "indexer", ev.IndexerID, "error", err)
	}
}

// NoteResults registers the results an indexer returned for a query, for
// the overlap comparison. keys identify releases across indexers (info
// hash, or a title+size fallback). Called for every answer served,
// including cache hits, so all indexers asked the same query are compared.
func (s *Stats) NoteResults(indexerID, queryKey string, keys []string) {
	if queryKey == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.groups[queryKey]
	if !ok {
		if len(s.groups) >= maxOverlapGroups {
			s.flushLocked(context.Background(), time.Time{}, true)
		}
		g = &overlapGroup{first: s.now(), byIndexer: make(map[string]map[string]struct{})}
		s.groups[queryKey] = g
	}
	set := g.byIndexer[indexerID]
	if set == nil {
		set = make(map[string]struct{}, len(keys))
		g.byIndexer[indexerID] = set
	}
	for _, k := range keys {
		if k != "" {
			set[k] = struct{}{}
		}
	}
}

// FlushOverlap finalizes query groups whose window has closed.
func (s *Stats) FlushOverlap(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushLocked(ctx, s.now().Add(-overlapWindow), false)
}

// flushLocked finalizes groups that started before cutoff, or (oldestHalf)
// the older half of all groups to make room. Caller holds mu.
func (s *Stats) flushLocked(ctx context.Context, cutoff time.Time, oldestHalf bool) {
	if oldestHalf {
		starts := make([]time.Time, 0, len(s.groups))
		for _, g := range s.groups {
			starts = append(starts, g.first)
		}
		sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
		cutoff = starts[len(starts)/2].Add(time.Nanosecond)
	}
	at := s.now().UTC().Format(time.RFC3339)
	for key, g := range s.groups {
		if !g.first.Before(cutoff) {
			continue
		}
		delete(s.groups, key)
		if len(g.byIndexer) < 2 {
			continue // nothing to compare against
		}
		for id, own := range g.byIndexer {
			unique := 0
			for k := range own {
				seenElsewhere := false
				for other, set := range g.byIndexer {
					if other == id {
						continue
					}
					if _, ok := set[k]; ok {
						seenElsewhere = true
						break
					}
				}
				if !seenElsewhere {
					unique++
				}
			}
			err := s.q.InsertOverlapEvent(ctx, db.InsertOverlapEventParams{
				IndexerID:     id,
				At:            at,
				Results:       int64(len(own)),
				UniqueResults: int64(unique),
				ComparedWith:  int64(len(g.byIndexer) - 1),
			})
			if err != nil {
				s.logger.Warn("indexer stats: recording overlap failed", "indexer", id, "error", err)
			}
		}
	}
}

// Prune deletes stats older than the retention window.
func (s *Stats) Prune(ctx context.Context) {
	cutoff := s.now().Add(-statsRetention).UTC().Format(time.RFC3339)
	if err := s.q.PruneSearchEvents(ctx, cutoff); err != nil {
		s.logger.Warn("indexer stats: pruning searches failed", "error", err)
	}
	if err := s.q.PruneOverlapEvents(ctx, cutoff); err != nil {
		s.logger.Warn("indexer stats: pruning overlap failed", "error", err)
	}
}

// Run finalizes overlap groups every minute and prunes old rows daily,
// until ctx ends.
func (s *Stats) Run(ctx context.Context) {
	s.Prune(ctx)
	flush := time.NewTicker(time.Minute)
	prune := time.NewTicker(24 * time.Hour)
	defer flush.Stop()
	defer prune.Stop()
	for {
		select {
		case <-flush.C:
			s.FlushOverlap(ctx)
		case <-prune.C:
			s.Prune(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// IndexerStats summarizes one indexer's searches over the stats window.
type IndexerStats struct {
	IndexerID string `json:"indexer_id"`
	// Searches counts real scrapes (cache hits excluded).
	Searches int `json:"searches"`
	Failures int `json:"failures"`
	// SuccessRate is successful searches / searches (0–1).
	SuccessRate float64 `json:"success_rate"`
	// MedianMs and P90Ms are search durations, successful searches only.
	MedianMs int `json:"median_ms"`
	P90Ms    int `json:"p90_ms"`
	// AvgResults and AvgWithSeeders are per successful search.
	AvgResults     float64 `json:"avg_results"`
	AvgWithSeeders float64 `json:"avg_with_seeders"`
	// MedianSeeders is the typical seed count of the best-seeded results
	// (median over searches that returned something).
	MedianSeeders int `json:"median_seeders"`
	// BehindCloudflare is the share of searches that met Cloudflare (0–1);
	// SolveRate is how many of those were solved (0–1).
	BehindCloudflare float64 `json:"behind_cloudflare"`
	SolveRate        float64 `json:"solve_rate"`
	// ErrorKinds counts failures by kind.
	ErrorKinds map[string]int `json:"error_kinds,omitempty"`
	// Comparisons is how many times this indexer answered a query alongside
	// others; UniqueShare is the share of its results nobody else returned
	// (0–1) and AvgUnique the number per comparison. Zero when never
	// compared.
	Comparisons int     `json:"comparisons"`
	UniqueShare float64 `json:"unique_share"`
	AvgUnique   float64 `json:"avg_unique"`
	LastSearch  string  `json:"last_search,omitempty"`
}

// Summarize returns stats for every indexer with any recorded activity in
// the retention window, keyed by indexer ID.
func (s *Stats) Summarize(ctx context.Context) (map[string]*IndexerStats, error) {
	since := s.now().Add(-statsRetention).UTC().Format(time.RFC3339)
	events, err := s.q.ListSearchEventsSince(ctx, since)
	if err != nil {
		return nil, err
	}
	overlaps, err := s.q.ListOverlapEventsSince(ctx, since)
	if err != nil {
		return nil, err
	}

	out := make(map[string]*IndexerStats)
	get := func(id string) *IndexerStats {
		st, ok := out[id]
		if !ok {
			st = &IndexerStats{IndexerID: id}
			out[id] = st
		}
		return st
	}

	type acc struct {
		durations, seeders           []int
		results, withSeeders, solved int
		challenged                   int
	}
	accs := make(map[string]*acc)
	for _, e := range events {
		st := get(e.IndexerID)
		a := accs[e.IndexerID]
		if a == nil {
			a = &acc{}
			accs[e.IndexerID] = a
		}
		st.Searches++
		st.LastSearch = e.At // rows are ordered by time
		if e.Challenged {
			a.challenged++
			if e.Solved {
				a.solved++
			}
		}
		if !e.Ok {
			st.Failures++
			if st.ErrorKinds == nil {
				st.ErrorKinds = make(map[string]int)
			}
			st.ErrorKinds[e.ErrorKind]++
			continue
		}
		a.durations = append(a.durations, int(e.DurationMs))
		a.results += int(e.Results)
		a.withSeeders += int(e.WithSeeders)
		if e.Results > 0 {
			a.seeders = append(a.seeders, int(e.MedianSeeders))
		}
	}
	for id, a := range accs {
		st := out[id]
		ok := st.Searches - st.Failures
		st.SuccessRate = ratio(ok, st.Searches)
		st.MedianMs = percentile(a.durations, 50)
		st.P90Ms = percentile(a.durations, 90)
		st.AvgResults = ratio(a.results, ok)
		st.AvgWithSeeders = ratio(a.withSeeders, ok)
		st.MedianSeeders = percentile(a.seeders, 50)
		st.BehindCloudflare = ratio(a.challenged, st.Searches)
		st.SolveRate = ratio(a.solved, a.challenged)
	}

	type ovAcc struct{ results, unique int }
	ov := make(map[string]*ovAcc)
	for _, o := range overlaps {
		st := get(o.IndexerID)
		st.Comparisons++
		a := ov[o.IndexerID]
		if a == nil {
			a = &ovAcc{}
			ov[o.IndexerID] = a
		}
		a.results += int(o.Results)
		a.unique += int(o.UniqueResults)
	}
	for id, a := range ov {
		st := out[id]
		st.UniqueShare = ratio(a.unique, a.results)
		st.AvgUnique = ratio(a.unique, st.Comparisons)
	}
	return out, nil
}

func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

// percentile returns the p-th percentile (nearest rank) of xs, 0 if empty.
func percentile(xs []int, p int) int {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]int(nil), xs...)
	sort.Ints(sorted)
	rank := (p*len(sorted) + 99) / 100 // ceil(p/100 * n)
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

// topMedian is the median of the n largest values in xs.
func topMedian(xs []int, n int) int {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]int(nil), xs...)
	sort.Sort(sort.Reverse(sort.IntSlice(sorted)))
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	return sorted[len(sorted)/2]
}
