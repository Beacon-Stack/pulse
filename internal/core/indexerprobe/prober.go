package indexerprobe

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/beacon-stack/pulse/internal/core/indexer"
	db "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/scraper"
)

// The tester runs one test search against each eligible catalog indexer per
// cycle, gently:
//
//   - eligible = public torrent indexers in a category a connected service
//     wants (Prism → Movies, Pilot → TV), not already added, needing no
//     settings without defaults. Private sites are never contacted.
//   - one search per site per cycle, one site at a time, spread across the
//     cycle with jitter; the same query for every site, so results compare.
//   - sites failing 3 cycles in a row drop to daily, and to weekly after a
//     week; rate-limited (HTTP 429) and Cloudflare sites to daily.
//   - a Cloudflare site waits while FlareSolverr is busy with a real search.
const (
	DefaultInterval = 6 * time.Hour
	StartDelay      = 10 * time.Minute

	probeTimeout    = 3 * time.Minute
	manualCooldown  = 10 * time.Minute
	retention       = 30 * 24 * time.Hour
	failingAfter    = 3
	weeklyAfter     = 7 * 24 * time.Hour
	slowSiteEvery   = 24 * time.Hour
	solverWaitMax   = 10 * time.Minute
	solverPoll      = 15 * time.Second
	minSpacing      = 20 * time.Second
	maxSpacing      = 15 * time.Minute
	recentQueryPool = 20
	maxErrorMessage = 300
)

// fallbackQueries are used until Prism or Pilot have searched for something.
var fallbackQueries = map[string][]string{
	"movie": {"the matrix", "inception", "interstellar", "the dark knight", "pulp fiction", "gladiator"},
	"tv":    {"breaking bad", "the office", "game of thrones", "friends", "the simpsons", "stranger things"},
}

// ErrCooldown is returned by TestNow when the site was tested too recently.
var ErrCooldown = errors.New("tested less than 10 minutes ago")

// Searcher is the part of the scraper engine the tester needs.
type Searcher interface {
	ProbeSearch(ctx context.Context, catalogID, query string) ([]scraper.SearchResult, *scraper.Trace, error)
	SolverBusy() bool
	HasDefinition(catalogID string) bool
	ResolveCatalogID(name, url string) string
}

// Status is what the tester is doing, for the add-indexer page.
type Status struct {
	Running          bool     `json:"running"`
	CycleStartedAt   string   `json:"cycle_started_at,omitempty"`
	LastCycleEndedAt string   `json:"last_cycle_ended_at,omitempty"`
	NextCycleAt      string   `json:"next_cycle_at,omitempty"`
	Query            string   `json:"query,omitempty"`
	Total            int      `json:"total"`
	Done             int      `json:"done"`
	IntervalHours    int      `json:"interval_hours"`
	WantedCategories []string `json:"wanted_categories"`
}

// Prober tests catalog indexers in the background.
type Prober struct {
	q        db.Querier
	search   Searcher
	catalog  func() []indexer.CatalogEntry
	logger   *slog.Logger
	interval time.Duration

	// Swappable in tests.
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) bool
	rng   *rand.Rand

	probeMu sync.Mutex // one test search at a time, cycle or manual

	mu        sync.Mutex
	status    Status
	lastProbe map[string]time.Time // per site, for the manual cooldown
	cycleNum  int
}

// New returns a tester. catalog lists the catalog entries (indexer.Catalog).
func New(q db.Querier, search Searcher, catalog func() []indexer.CatalogEntry, logger *slog.Logger) *Prober {
	return &Prober{
		q:         q,
		search:    search,
		catalog:   catalog,
		logger:    logger,
		interval:  DefaultInterval,
		now:       time.Now,
		sleep:     sleepCtx,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
		lastProbe: make(map[string]time.Time),
		status:    Status{IntervalHours: int(DefaultInterval / time.Hour)},
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// Status returns a snapshot of what the tester is doing.
func (p *Prober) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.status
	s.WantedCategories = append([]string(nil), s.WantedCategories...)
	return s
}

// Run tests catalog indexers every interval until ctx ends, starting after
// StartDelay so a restart doesn't add load while everything boots.
func (p *Prober) Run(ctx context.Context) {
	p.prune(ctx)
	p.setNext(p.now().Add(StartDelay))
	if !p.sleep(ctx, StartDelay) {
		return
	}
	for {
		started := p.now()
		p.RunCycle(ctx)
		p.prune(ctx)
		next := started.Add(p.interval)
		p.setNext(next)
		if !p.sleep(ctx, next.Sub(p.now())) {
			return
		}
	}
}

func (p *Prober) setNext(t time.Time) {
	p.mu.Lock()
	p.status.NextCycleAt = t.UTC().Format(time.RFC3339)
	p.mu.Unlock()
}

// Eligible returns the catalog indexers the tester covers, the categories
// connected services want, and the catalog IDs already added.
func (p *Prober) Eligible(ctx context.Context) (targets []indexer.CatalogEntry, wanted []string, added map[string]bool, err error) {
	wantedSet := map[string]bool{}
	for _, cat := range []string{"Movies", "TV", "Audio", "Books", "XXX", "Other"} {
		capability := indexer.CapabilityForCategory(cat)
		if capability == "" {
			continue
		}
		svcs, err := p.q.ListServicesByCapability(ctx, capability)
		if err != nil {
			return nil, nil, nil, err
		}
		if len(svcs) > 0 {
			wantedSet[cat] = true
			wanted = append(wanted, cat)
		}
	}

	rows, err := p.q.ListIndexers(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	added = make(map[string]bool, len(rows))
	for _, r := range rows {
		if id := p.search.ResolveCatalogID(r.Name, r.Url); id != "" {
			added[id] = true
		}
	}

	for _, e := range p.catalog() {
		if e.Privacy != "public" || e.Protocol != "torrent" || added[e.ID] || needsSetup(e) {
			continue
		}
		if !anyWanted(e.Categories, wantedSet) || !p.search.HasDefinition(e.ID) {
			continue
		}
		targets = append(targets, e)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })
	return targets, wanted, added, nil
}

func anyWanted(cats []string, wanted map[string]bool) bool {
	for _, c := range cats {
		if wanted[c] {
			return true
		}
	}
	return false
}

// needsSetup reports whether adding the indexer requires a value it has no
// default for (an account, a cookie...). Those can't be tested blind.
func needsSetup(e indexer.CatalogEntry) bool {
	for _, f := range e.Settings {
		if f.Required && f.Default == "" && !strings.HasPrefix(f.Type, "info") {
			return true
		}
	}
	return false
}

// siteHistory is what past tests say about a site, for scheduling.
type siteHistory struct {
	last, failingSince time.Time
	consecutiveFails   int
	lastErrorKind      string
	cloudflare         bool
}

func (p *Prober) histories(ctx context.Context) (map[string]*siteHistory, error) {
	since := p.now().Add(-retention).UTC().Format(time.RFC3339)
	rows, err := p.q.ListCatalogProbesSince(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*siteHistory)
	for _, r := range rows { // ordered by catalog_id, at
		h := out[r.CatalogID]
		if h == nil {
			h = &siteHistory{}
			out[r.CatalogID] = h
		}
		at, _ := time.Parse(time.RFC3339, r.At)
		h.last = at
		if r.Challenged {
			h.cloudflare = true
		}
		if r.Ok {
			h.consecutiveFails, h.failingSince, h.lastErrorKind = 0, time.Time{}, ""
			continue
		}
		if h.consecutiveFails == 0 {
			h.failingSince = at
		}
		h.consecutiveFails++
		h.lastErrorKind = r.ErrorKind
	}
	return out, nil
}

// due reports whether a site should be tested this cycle.
func due(h *siteHistory, now time.Time) bool {
	if h == nil || h.last.IsZero() {
		return true
	}
	since := now.Sub(h.last)
	switch {
	case h.consecutiveFails >= failingAfter && now.Sub(h.failingSince) >= weeklyAfter:
		return since >= 7*24*time.Hour
	case h.consecutiveFails >= failingAfter, h.lastErrorKind == "rate_limited", h.cloudflare:
		return since >= slowSiteEvery
	default:
		return true
	}
}

// pickQuery chooses this cycle's test query: a recent real search, from
// movies and TV in turn (whichever the connected services want), else a
// well-known title.
func (p *Prober) pickQuery(ctx context.Context, wanted []string) string {
	var modes []string
	for _, c := range wanted {
		switch c {
		case "Movies":
			modes = append(modes, "movie")
		case "TV":
			modes = append(modes, "tv")
		}
	}
	if len(modes) == 0 {
		modes = []string{"movie"}
	}
	p.mu.Lock()
	mode := modes[p.cycleNum%len(modes)]
	p.cycleNum++
	p.mu.Unlock()

	rows, err := p.q.ListRecentQueries(ctx, db.ListRecentQueriesParams{Mode: mode, Limit: recentQueryPool})
	var pool []string
	if err == nil {
		for _, r := range rows {
			if usableQuery(r.Query) {
				pool = append(pool, r.Query)
			}
		}
	}
	if len(pool) == 0 {
		pool = fallbackQueries[mode]
	}
	return pool[p.rng.Intn(len(pool))]
}

// usableQuery rejects queries that only mean something to one site, such
// as bare IMDb IDs.
func usableQuery(q string) bool {
	q = strings.TrimSpace(q)
	if len(q) < 3 {
		return false
	}
	if strings.HasPrefix(q, "tt") && strings.Trim(q[2:], "0123456789") == "" {
		return false
	}
	return true
}

// RunCycle tests every eligible site that is due, once.
func (p *Prober) RunCycle(ctx context.Context) {
	targets, wanted, _, err := p.Eligible(ctx)
	if err != nil {
		p.logger.Warn("catalog tester: listing indexers failed", "error", err)
		return
	}
	hist, err := p.histories(ctx)
	if err != nil {
		p.logger.Warn("catalog tester: reading history failed", "error", err)
		return
	}
	now := p.now()
	var dueTargets []indexer.CatalogEntry
	for _, t := range targets {
		if due(hist[t.ID], now) {
			dueTargets = append(dueTargets, t)
		}
	}
	p.rng.Shuffle(len(dueTargets), func(i, j int) { dueTargets[i], dueTargets[j] = dueTargets[j], dueTargets[i] })

	query := p.pickQuery(ctx, wanted)
	cycleID := now.UTC().Format(time.RFC3339)
	p.mu.Lock()
	p.status.Running = true
	p.status.CycleStartedAt = cycleID
	p.status.Query = query
	p.status.Total = len(dueTargets)
	p.status.Done = 0
	p.status.WantedCategories = wanted
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.status.Running = false
		p.status.LastCycleEndedAt = p.now().UTC().Format(time.RFC3339)
		p.mu.Unlock()
	}()
	p.logger.Info("catalog tester: cycle starting", "sites", len(dueTargets), "eligible", len(targets), "query", query)

	spacing := minSpacing
	if len(dueTargets) > 0 {
		spacing = p.interval * 3 / 4 / time.Duration(len(dueTargets))
	}
	spacing = min(max(spacing, minSpacing), maxSpacing)

	found := make(map[string][]indexer.ReleaseID)
	rowIDs := make(map[string]int64)
	for i, t := range dueTargets {
		if i > 0 && !p.sleep(ctx, p.jitter(spacing)) {
			return
		}
		if h := hist[t.ID]; h != nil && h.cloudflare && !p.waitForSolver(ctx) {
			return
		}
		res, rowID, err := p.probe(ctx, t.ID, query, cycleID, false)
		if errors.Is(err, errProxyDown) {
			p.logger.Warn("catalog tester: VPN proxy is down, stopping this cycle")
			return
		}
		if err == nil && rowID > 0 {
			rowIDs[t.ID] = rowID
			ids := make([]indexer.ReleaseID, len(res))
			for j, r := range res {
				ids[j] = ReleaseIDOf(r)
			}
			found[t.ID] = ids
		}
		p.mu.Lock()
		p.status.Done++
		p.mu.Unlock()
	}

	for id, n := range indexer.UniqueCounts(found) {
		if err := p.q.SetCatalogProbeUnique(ctx, db.SetCatalogProbeUniqueParams{UniqueResults: int64(n), ID: rowIDs[id]}); err != nil {
			p.logger.Warn("catalog tester: saving unique count failed", "site", id, "error", err)
		}
	}
	p.logger.Info("catalog tester: cycle done", "sites", len(dueTargets))
}

func (p *Prober) jitter(d time.Duration) time.Duration {
	// ±25%
	return d*3/4 + time.Duration(p.rng.Int63n(int64(d)/2+1))
}

// waitForSolver waits while FlareSolverr serves a real search, up to
// solverWaitMax. False if ctx ended.
func (p *Prober) waitForSolver(ctx context.Context) bool {
	deadline := p.now().Add(solverWaitMax)
	for p.search.SolverBusy() && p.now().Before(deadline) {
		if !p.sleep(ctx, solverPoll) {
			return false
		}
	}
	return ctx.Err() == nil
}

var errProxyDown = errors.New("scraper proxy down")

// probe runs and records one test search. A failure caused by our own VPN
// proxy being down says nothing about the site and is not recorded.
func (p *Prober) probe(ctx context.Context, catalogID, query, cycleID string, manual bool) ([]scraper.SearchResult, int64, error) {
	p.probeMu.Lock()
	defer p.probeMu.Unlock()

	pctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	started := p.now()
	results, trace, err := p.search.ProbeSearch(pctx, catalogID, query)
	elapsed := p.now().Sub(started)
	if ctx.Err() != nil {
		return nil, 0, ctx.Err() // shutting down; not the site's fault
	}
	kind := ErrorKind(err)
	if kind == "proxy_down" {
		return nil, 0, errProxyDown
	}

	p.mu.Lock()
	p.lastProbe[catalogID] = p.now()
	p.mu.Unlock()

	seeders := make([]int, len(results))
	withSeeders := 0
	for i, r := range results {
		seeders[i] = r.Seeders
		if r.Seeders > 0 {
			withSeeders++
		}
	}
	challenged, solved := false, false
	if trace != nil {
		challenged, solved = trace.Challenge()
	}
	msg := ""
	if err != nil {
		msg = err.Error()
		if len(msg) > maxErrorMessage {
			msg = msg[:maxErrorMessage]
		}
	}
	rowID, dbErr := p.q.InsertCatalogProbe(ctx, db.InsertCatalogProbeParams{
		CatalogID:     catalogID,
		CycleID:       cycleID,
		At:            started.UTC().Format(time.RFC3339),
		Query:         query,
		Manual:        manual,
		DurationMs:    elapsed.Milliseconds(),
		Ok:            err == nil,
		ErrorKind:     kind,
		ErrorMessage:  msg,
		Results:       int64(len(results)),
		WithSeeders:   int64(withSeeders),
		MedianSeeders: int64(indexer.TopMedian(seeders)),
		Challenged:    challenged,
		Solved:        challenged && solved,
	})
	if dbErr != nil {
		p.logger.Warn("catalog tester: saving result failed", "site", catalogID, "error", dbErr)
	}
	if err != nil {
		p.logger.Info("catalog tester: site failed", "site", catalogID, "kind", kind, "error", msg)
	}
	return results, rowID, err
}

// TestNow tests one catalog indexer immediately (the "Test now" button).
// Limited to once per 10 minutes per site.
func (p *Prober) TestNow(ctx context.Context, catalogID string) error {
	var entry *indexer.CatalogEntry
	for _, e := range p.catalog() {
		if e.ID == catalogID {
			entry = &e
			break
		}
	}
	if entry == nil || !p.search.HasDefinition(catalogID) {
		return fmt.Errorf("unknown indexer %q", catalogID)
	}
	if entry.Privacy != "public" {
		return fmt.Errorf("%s needs an account, so it can't be tested before adding it", entry.Name)
	}
	p.mu.Lock()
	last := p.lastProbe[catalogID]
	p.mu.Unlock()
	if !last.IsZero() && p.now().Sub(last) < manualCooldown {
		return ErrCooldown
	}
	_, wanted, _, err := p.Eligible(ctx)
	if err != nil {
		return err
	}
	_, _, err = p.probe(ctx, catalogID, p.pickQuery(ctx, wanted), "manual", true)
	if errors.Is(err, errProxyDown) {
		return errors.New("the VPN proxy is down, so nothing could be tested")
	}
	return nil // a failing site is a result, not an error
}

func (p *Prober) prune(ctx context.Context) {
	cutoff := p.now().Add(-retention).UTC().Format(time.RFC3339)
	if err := p.q.PruneCatalogProbes(ctx, cutoff); err != nil {
		p.logger.Warn("catalog tester: pruning failed", "error", err)
	}
}
