package indexerprobe

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/beacon-stack/pulse/internal/core/indexer"
	pulsedb "github.com/beacon-stack/pulse/internal/db"
	db "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/scraper"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeSearch struct {
	mu       sync.Mutex
	results  map[string][]scraper.SearchResult
	errs     map[string]error
	cf       map[string]bool
	defs     map[string]bool
	calls    []string // "site:query"
	busy     int      // SolverBusy returns true this many times
	resolved map[string]string
}

func (f *fakeSearch) ProbeSearch(_ context.Context, id, q string) ([]scraper.SearchResult, *scraper.Trace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, id+":"+q)
	tr := &scraper.Trace{}
	if f.cf[id] {
		markChallenge(tr)
	}
	return f.results[id], tr, f.errs[id]
}

func (f *fakeSearch) SolverBusy() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busy > 0 {
		f.busy--
		return true
	}
	return false
}
func (f *fakeSearch) HasDefinition(id string) bool           { return f.defs[id] }
func (f *fakeSearch) ResolveCatalogID(name, _ string) string { return f.resolved[name] }

// markChallenge records a solved Cloudflare challenge on a trace through
// the public API the runner uses.
func markChallenge(tr *scraper.Trace) {
	scraper.NoteChallengeForTest(tr, true)
}

func newDB(t *testing.T) *db.Queries {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := pulsedb.Migrate(sqlDB); err != nil {
		t.Fatal(err)
	}
	return db.New(sqlDB)
}

func addService(t *testing.T, q *db.Queries, id, capability string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := q.CreateService(context.Background(), db.CreateServiceParams{
		ID: id, Name: id, Type: "media-manager", ApiUrl: "http://" + id, HealthUrl: "http://" + id,
		Status: "online", LastSeen: now, Registered: now, Metadata: "{}",
	}); err != nil {
		t.Fatalf("creating service: %v", err)
	}
	if err := q.AddCapability(context.Background(), db.AddCapabilityParams{ID: id + "-cap", ServiceID: id, Capability: capability}); err != nil {
		t.Fatalf("adding capability: %v", err)
	}
}

var testCatalog = []indexer.CatalogEntry{
	{ID: "good", Name: "Good", Privacy: "public", Protocol: "torrent", Categories: []string{"Movies", "TV"}},
	{ID: "slowcf", Name: "SlowCF", Privacy: "public", Protocol: "torrent", Categories: []string{"Movies"}},
	{ID: "broken", Name: "Broken", Privacy: "public", Protocol: "torrent", Categories: []string{"Movies"}},
	{ID: "tvonly", Name: "TVOnly", Privacy: "public", Protocol: "torrent", Categories: []string{"TV"}},
	{ID: "private", Name: "Private", Privacy: "private", Protocol: "torrent", Categories: []string{"Movies"}},
	{ID: "setup", Name: "Setup", Privacy: "public", Protocol: "torrent", Categories: []string{"Movies"},
		Settings: []indexer.Field{{Name: "cookie", Type: "text", Required: true}}},
	{ID: "nodef", Name: "NoDef", Privacy: "public", Protocol: "torrent", Categories: []string{"Movies"}},
	{ID: "mine", Name: "Mine", Privacy: "public", Protocol: "torrent", Categories: []string{"Movies"}},
}

type harness struct {
	p      *Prober
	q      *db.Queries
	search *fakeSearch
	now    *time.Time
	sleeps []time.Duration
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	q := newDB(t)
	addService(t, q, "prism", "content:movies")
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	fs := &fakeSearch{
		results: map[string][]scraper.SearchResult{
			"good":   {{Title: "Dune 2021 1080p", InfoHash: "aaa", Seeders: 500}, {Title: "Dune 2021 720p", InfoHash: "bbb", Seeders: 80}},
			"slowcf": {{Title: "Dune 2021 1080p", InfoHash: "aaa", Seeders: 450}, {Title: "Dune 2021 Remux", InfoHash: "ccc", Seeders: 12}},
		},
		errs:     map[string]error{"broken": errors.New("HTTP 502")},
		cf:       map[string]bool{"slowcf": true},
		defs:     map[string]bool{"good": true, "slowcf": true, "broken": true, "tvonly": true, "private": true, "setup": true, "mine": true},
		resolved: map[string]string{"Mine": "mine"},
	}
	now0 := time.Now().UTC().Format(time.RFC3339)
	if _, err := q.CreateIndexer(context.Background(), db.CreateIndexerParams{
		ID: "idx-mine", Name: "Mine", Kind: "torznab", Enabled: true, Priority: 25,
		Url: "https://mine.example", Settings: "{}", CreatedAt: now0, UpdatedAt: now0,
	}); err != nil {
		t.Fatal(err)
	}
	h := &harness{q: q, search: fs, now: &now}
	h.p = New(q, fs, func() []indexer.CatalogEntry { return testCatalog }, slog.Default())
	h.p.now = func() time.Time { return *h.now }
	h.p.rng = rand.New(rand.NewSource(1))
	h.p.sleep = func(ctx context.Context, d time.Duration) bool {
		h.sleeps = append(h.sleeps, d)
		*h.now = h.now.Add(d)
		return ctx.Err() == nil
	}
	return h
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestEligible(t *testing.T) {
	h := newHarness(t)
	targets, wanted, added, err := h.p.Eligible(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range targets {
		ids = append(ids, e.ID)
	}
	// Movies only (no TV service); not private, needing setup, without a
	// definition, or already added.
	if fmt.Sprint(ids) != "[broken good slowcf]" {
		t.Errorf("eligible = %v, want [broken good slowcf]", ids)
	}
	if fmt.Sprint(wanted) != "[Movies]" {
		t.Errorf("wanted = %v", wanted)
	}
	if !added["mine"] {
		t.Errorf("added = %v, want mine", added)
	}
}

func TestStatusListsAreNeverNull(t *testing.T) {
	h := newHarness(t)
	b, err := json.Marshal(h.p.Status()) // before any round has run
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "null") {
		t.Errorf("status JSON has a null: %s", b)
	}
}

func TestRunCycle(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.p.RunCycle(ctx)

	// Every eligible site searched once, all with the same query.
	if len(h.search.calls) != 3 {
		t.Fatalf("calls = %v, want 3", h.search.calls)
	}
	queries := map[string]bool{}
	for _, c := range h.search.calls {
		queries[strings.SplitN(c, ":", 2)[1]] = true
	}
	if len(queries) != 1 {
		t.Errorf("sites got different queries: %v", h.search.calls)
	}
	// One at a time, ~90 s apart.
	if len(h.sleeps) != 2 {
		t.Errorf("sleeps = %v, want 2 gaps", h.sleeps)
	}
	for _, d := range h.sleeps {
		if d < probeSpacing*3/4 || d > probeSpacing*5/4 {
			t.Errorf("gap %v outside 90s ±25%%", d)
		}
	}
	st := h.p.Status()
	if st.Running || st.Total != 3 || st.Done != 3 {
		t.Errorf("status = %+v", st)
	}

	health, err := h.p.Health(ctx)
	if err != nil {
		t.Fatal(err)
	}
	good, cf, broken := health["good"], health["slowcf"], health["broken"]
	if good.Verdict != VerdictWorks || cf.Verdict != VerdictWorks || broken.Verdict != VerdictDown {
		t.Errorf("verdicts = %s/%s/%s", good.Verdict, cf.Verdict, broken.Verdict)
	}
	// good found aaa,bbb; slowcf found aaa,ccc: one unique each.
	if good.Comparisons != 1 || good.UniqueShare != 0.5 || cf.UniqueShare != 0.5 {
		t.Errorf("unique: good %+v, slowcf %+v", good, cf)
	}
	if cf.BehindCloudflare != 1 || cf.SolveRate != 1 {
		t.Errorf("slowcf cloudflare = %v/%v", cf.BehindCloudflare, cf.SolveRate)
	}
	if good.Score <= cf.Score || cf.Score <= broken.Score {
		t.Errorf("scores good %d, slowcf %d, broken %d: want good > slowcf > broken", good.Score, cf.Score, broken.Score)
	}
	if broken.LastErrorKind != "http_error" || broken.LastError != "HTTP 502" {
		t.Errorf("broken error = %q/%q", broken.LastErrorKind, broken.LastError)
	}
}

func TestBackoffSchedule(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		h    *siteHistory
		want bool
	}{
		{"never tested", nil, true},
		{"healthy, tested last cycle", &siteHistory{last: now.Add(-6 * time.Hour)}, true},
		{"healthy, tested an hour ago (Pulse restarted)", &siteHistory{last: now.Add(-time.Hour)}, false},
		{"cloudflare, 6h ago", &siteHistory{last: now.Add(-6 * time.Hour), cloudflare: true}, false},
		{"cloudflare, a day ago", &siteHistory{last: now.Add(-25 * time.Hour), cloudflare: true}, true},
		{"rate limited, 6h ago", &siteHistory{last: now.Add(-6 * time.Hour), consecutiveFails: 1, lastErrorKind: "rate_limited"}, false},
		{"2 failures: keep trying", &siteHistory{last: now.Add(-6 * time.Hour), consecutiveFails: 2, failingSince: now.Add(-12 * time.Hour)}, true},
		{"3 failures: daily", &siteHistory{last: now.Add(-6 * time.Hour), consecutiveFails: 3, failingSince: now.Add(-18 * time.Hour)}, false},
		{"3 failures, a day later", &siteHistory{last: now.Add(-24 * time.Hour), consecutiveFails: 3, failingSince: now.Add(-36 * time.Hour)}, true},
		{"failing 8 days: weekly", &siteHistory{last: now.Add(-2 * 24 * time.Hour), consecutiveFails: 9, failingSince: now.Add(-8 * 24 * time.Hour)}, false},
		{"failing 8 days, a week later", &siteHistory{last: now.Add(-7 * 24 * time.Hour), consecutiveFails: 9, failingSince: now.Add(-15 * 24 * time.Hour)}, true},
	}
	for _, c := range cases {
		if got := due(c.h, now); got != c.want {
			t.Errorf("%s: due = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBrokenSiteBacksOff(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		h.p.RunCycle(ctx)
		*h.now = h.now.Add(DefaultInterval)
	}
	n := 0
	for _, c := range h.search.calls {
		if c[:len("broken:")] == "broken:" {
			n++
		}
	}
	// Tested in cycles 1–3, then skipped until a day has passed.
	if n != 3 {
		t.Errorf("broken site tested %d times in 4 cycles, want 3", n)
	}
}

func TestPickQuery(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	stats := indexer.NewStats(h.q, slog.Default())
	stats.RecordSearch(ctx, indexer.SearchEvent{IndexerID: "idx-mine", At: *h.now, Duration: time.Second, Query: "tt0133093", Mode: "movie"})
	stats.RecordSearch(ctx, indexer.SearchEvent{IndexerID: "idx-mine", At: *h.now, Duration: time.Second, Query: "nosferatu 1922", Mode: "movie"})
	stats.RecordSearch(ctx, indexer.SearchEvent{IndexerID: "idx-mine", At: *h.now, Duration: time.Second, Query: "severance S02E01", Mode: "tv"})

	if got := h.p.pickQuery(ctx, []string{"Movies"}); got != "nosferatu 1922" {
		t.Errorf("movie query = %q, want the recent real search (not the IMDb ID)", got)
	}
	// Movies and TV alternate between cycles.
	h.p.cycleNum = 1
	if got := h.p.pickQuery(ctx, []string{"Movies", "TV"}); got != "severance S02E01" {
		t.Errorf("second cycle query = %q, want the TV search", got)
	}
	// No real searches yet: a well-known title.
	empty := newHarness(t)
	got := empty.p.pickQuery(ctx, []string{"Movies"})
	found := false
	for _, f := range fallbackQueries["movie"] {
		found = found || f == got
	}
	if !found {
		t.Errorf("fallback query = %q", got)
	}
}

func TestRestartResumesRound(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.p.RunCycle(ctx)
	first := len(h.search.calls)
	// Pulse restarts an hour later: nothing tested is due again yet.
	*h.now = h.now.Add(time.Hour)
	h.p.RunCycle(ctx)
	if got := len(h.search.calls) - first; got != 0 {
		t.Errorf("restart re-tested %d sites, want 0 (calls %v)", got, h.search.calls)
	}
	// A new site appearing (e.g. a newly connected service) is tested at once.
	testCatalog = append(testCatalog, indexer.CatalogEntry{ID: "fresh", Name: "Fresh", Privacy: "public", Protocol: "torrent", Categories: []string{"Movies"}})
	defer func() { testCatalog = testCatalog[:len(testCatalog)-1] }()
	h.search.defs["fresh"] = true
	*h.now = h.now.Add(time.Hour)
	h.p.RunCycle(ctx)
	if last := h.search.calls[len(h.search.calls)-1]; !strings.HasPrefix(last, "fresh:") || len(h.search.calls)-first != 1 {
		t.Errorf("after adding a site, calls = %v; want only the new site tested", h.search.calls[first:])
	}
}

func TestCloudflareSiteWaitsForSolver(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.p.RunCycle(ctx) // learn that slowcf is behind Cloudflare
	*h.now = h.now.Add(25 * time.Hour)
	h.sleeps = nil
	h.search.busy = 2
	h.p.RunCycle(ctx)
	polls := 0
	for _, d := range h.sleeps {
		if d == solverPoll {
			polls++
		}
	}
	if polls != 2 {
		t.Errorf("waited %d polls for FlareSolverr, want 2 (sleeps %v)", polls, h.sleeps)
	}
}

func TestProxyDownStopsCycleWithoutBlamingSites(t *testing.T) {
	h := newHarness(t)
	down := errors.New(`HTTP request failed: Get "https://x/": proxyconnect tcp: dial tcp: connection refused`)
	for _, id := range []string{"good", "slowcf", "broken"} {
		h.search.errs[id] = down
	}
	h.p.RunCycle(context.Background())
	if len(h.search.calls) != 1 {
		t.Errorf("calls = %v, want the cycle to stop after the first", h.search.calls)
	}
	health, _ := h.p.Health(context.Background())
	if len(health) != 0 {
		t.Errorf("recorded %v; a proxy outage must not count against sites", health)
	}
}

func TestTestNow(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.p.TestNow(ctx, "good"); err != nil {
		t.Fatal(err)
	}
	if err := h.p.TestNow(ctx, "good"); !errors.Is(err, ErrCooldown) {
		t.Errorf("second test within 10 min: err = %v, want ErrCooldown", err)
	}
	*h.now = h.now.Add(manualCooldown)
	if err := h.p.TestNow(ctx, "good"); err != nil {
		t.Errorf("after the cooldown: %v", err)
	}
	if err := h.p.TestNow(ctx, "private"); err == nil {
		t.Error("testing a private indexer should be refused")
	}
	if err := h.p.TestNow(ctx, "nope"); err == nil {
		t.Error("testing an unknown indexer should fail")
	}
	// A failing site is a result, not an error.
	if err := h.p.TestNow(ctx, "broken"); err != nil {
		t.Errorf("failing site: %v", err)
	}
}

func TestVerdicts(t *testing.T) {
	cases := []struct {
		h    SiteHealth
		want string
	}{
		{SiteHealth{AvgResults: 10}, VerdictWorks},
		{SiteHealth{AvgResults: 0}, VerdictNoResults},
		{SiteHealth{ConsecutiveFailures: 1, LastOKAt: "x", LastErrorKind: "timeout"}, VerdictFlaky},
		{SiteHealth{ConsecutiveFailures: 1, LastErrorKind: "timeout"}, VerdictDown},
		{SiteHealth{ConsecutiveFailures: 3, LastOKAt: "x", LastErrorKind: "timeout"}, VerdictDown},
		{SiteHealth{ConsecutiveFailures: 1, LastOKAt: "x", LastErrorKind: "cloudflare_no_solver"}, VerdictNeedsFlareSolverr},
		{SiteHealth{ConsecutiveFailures: 1, LastOKAt: "x", LastErrorKind: "ip_banned"}, VerdictBlocked},
	}
	for _, c := range cases {
		h := c.h
		if got := verdict(&h); got != c.want {
			t.Errorf("%+v: verdict = %s, want %s", c.h, got, c.want)
		}
	}
}

func TestScoreOrdering(t *testing.T) {
	mk := func(seeders, ms int, unique float64, cf float64) *SiteHealth {
		h := &SiteHealth{Verdict: VerdictWorks, SuccessRate: 1, MedianSeeders: seeders, MedianMs: ms, Comparisons: 1, UniqueShare: unique, BehindCloudflare: cf}
		h.Score = score(h)
		return h
	}
	bigFast := mk(800, 1500, 0.3, 0)
	smallFast := mk(5, 1500, 0.3, 0)
	bigSlowCF := mk(800, 45000, 0.3, 1)
	if bigFast.Score <= bigSlowCF.Score || bigFast.Score <= smallFast.Score {
		t.Errorf("scores: big+fast %d, big+slow+CF %d, small+fast %d", bigFast.Score, bigSlowCF.Score, smallFast.Score)
	}
	if bigFast.Score > 100 || smallFast.Score < 1 {
		t.Errorf("scores out of range: %d %d", bigFast.Score, smallFast.Score)
	}
}
