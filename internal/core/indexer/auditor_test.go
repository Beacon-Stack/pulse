package indexer

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	pulsedb "github.com/beacon-stack/pulse/internal/db"
	db "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/events"
)

func newTestDB(t *testing.T) *db.Queries {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening in-memory db: %v", err)
	}
	// Each pooled connection to ":memory:" opens its own empty database,
	// so pin the pool to one connection or queries can miss the schema.
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := pulsedb.Migrate(sqlDB); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	return db.New(sqlDB)
}

func createTestIndexer(t *testing.T, q *db.Queries, id, name string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := q.CreateIndexer(context.Background(), db.CreateIndexerParams{
		ID: id, Name: name, Kind: "torznab", Enabled: true, Priority: 25,
		Url: "https://example.org", Settings: "{}", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("creating indexer %s: %v", name, err)
	}
}

// eventRecorder collects bus events for assertions. Bus handlers run in
// goroutines, so access is synchronized and tests poll via wait.
type eventRecorder struct {
	mu     sync.Mutex
	events []events.Event
}

func (r *eventRecorder) handle(_ context.Context, e events.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *eventRecorder) ofType(t events.Type) []events.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []events.Event
	for _, e := range r.events {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

func (r *eventRecorder) waitFor(t *testing.T, typ events.Type, want int) []events.Event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := r.ofType(typ); len(got) >= want {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	got := r.ofType(typ)
	t.Fatalf("timed out waiting for %d %s events, got %d", want, typ, len(got))
	return got
}

func newTestManager(t *testing.T) (*Manager, *db.Queries, *eventRecorder) {
	t.Helper()
	q := newTestDB(t)
	logger := slog.New(slog.DiscardHandler)
	bus := events.New(logger)
	rec := &eventRecorder{}
	bus.Subscribe(rec.handle)
	return NewManager(q, bus, logger), q, rec
}

// The 2026-08-16 incident: TorrentGalaxyClone's Prowlarr definition was
// removed upstream, and nothing flagged the configured indexer — every
// search just failed. AuditCatalog must mark it definition_missing and
// publish a health-changed event.
func TestAuditCatalog_FlagsMissingDefinition(t *testing.T) {
	mgr, q, rec := newTestManager(t)
	createTestIndexer(t, q, "id-tgx", "TorrentGalaxyClone")
	createTestIndexer(t, q, "id-1337x", "1337x")

	resolve := func(name, _ string) string {
		if name == "1337x" {
			return "1337x"
		}
		return ""
	}
	mgr.AuditCatalog(context.Background(), resolve)

	tgx, err := q.GetIndexer(context.Background(), "id-tgx")
	if err != nil {
		t.Fatal(err)
	}
	if tgx.HealthStatus != HealthDefinitionMissing {
		t.Errorf("TorrentGalaxyClone health = %q, want %q", tgx.HealthStatus, HealthDefinitionMissing)
	}
	if tgx.HealthMessage == "" {
		t.Error("TorrentGalaxyClone health message should explain the missing definition")
	}
	if tgx.HealthCheckedAt == "" {
		t.Error("health_checked_at should be stamped")
	}

	ok, err := q.GetIndexer(context.Background(), "id-1337x")
	if err != nil {
		t.Fatal(err)
	}
	if ok.HealthStatus != HealthOK {
		t.Errorf("1337x health = %q, want %q", ok.HealthStatus, HealthOK)
	}
	if ok.HealthMessage != "" {
		t.Errorf("1337x health message = %q, want empty", ok.HealthMessage)
	}

	// Both indexers transitioned from unknown, so both publish an event.
	rec.waitFor(t, events.TypeIndexerHealthChanged, 2)
}

// A repeat audit with no transitions must not re-publish events, but must
// still refresh health_checked_at.
func TestAuditCatalog_NoEventWithoutTransition(t *testing.T) {
	mgr, q, rec := newTestManager(t)
	createTestIndexer(t, q, "id-tgx", "TorrentGalaxyClone")

	noneResolve := func(string, string) string { return "" }
	mgr.AuditCatalog(context.Background(), noneResolve)
	first := rec.waitFor(t, events.TypeIndexerHealthChanged, 1)

	mgr.AuditCatalog(context.Background(), noneResolve)
	time.Sleep(50 * time.Millisecond) // give a spurious event time to arrive
	if got := rec.ofType(events.TypeIndexerHealthChanged); len(got) != len(first) {
		t.Errorf("repeat audit published %d extra events, want 0", len(got)-len(first))
	}
}

// Recovery path: a definition that reappears flips the indexer back to ok
// and publishes another event.
func TestAuditCatalog_RecoveryFlipsBackToOK(t *testing.T) {
	mgr, q, rec := newTestManager(t)
	createTestIndexer(t, q, "id-tgx", "TorrentGalaxyClone")

	mgr.AuditCatalog(context.Background(), func(string, string) string { return "" })
	rec.waitFor(t, events.TypeIndexerHealthChanged, 1)

	mgr.AuditCatalog(context.Background(), func(string, string) string { return "torrentgalaxyclone" })
	rec.waitFor(t, events.TypeIndexerHealthChanged, 2)

	row, err := q.GetIndexer(context.Background(), "id-tgx")
	if err != nil {
		t.Fatal(err)
	}
	if row.HealthStatus != HealthOK {
		t.Errorf("health after recovery = %q, want %q", row.HealthStatus, HealthOK)
	}
}

// Three consecutive search failures flip an indexer to failing; one
// success flips it back to ok. This is the live-search health dimension
// that AuditCatalog (definition dimension) cannot see — an indexer
// 400-ing on every search used to stay 'ok' forever.
func TestRecordSearchOutcome_FailingThresholdAndRecovery(t *testing.T) {
	mgr, q, rec := newTestManager(t)
	createTestIndexer(t, q, "id-x", "SomeIndexer")
	ctx := context.Background()

	boom := errors.New("HTTP 400")
	mgr.RecordSearchOutcome(ctx, "id-x", boom)
	mgr.RecordSearchOutcome(ctx, "id-x", boom)

	row, _ := q.GetIndexer(ctx, "id-x")
	if row.HealthStatus == HealthFailing {
		t.Fatalf("flipped to failing after only 2 failures, threshold is %d", failingThreshold)
	}
	if row.FailureCount != 2 {
		t.Errorf("failure_count = %d, want 2", row.FailureCount)
	}

	mgr.RecordSearchOutcome(ctx, "id-x", boom)
	row, _ = q.GetIndexer(ctx, "id-x")
	if row.HealthStatus != HealthFailing {
		t.Fatalf("health = %q after %d failures, want %q", row.HealthStatus, failingThreshold, HealthFailing)
	}
	if row.LastError == "" {
		t.Error("last_error should record the failure")
	}
	rec.waitFor(t, events.TypeIndexerHealthChanged, 1)

	// Recovery.
	mgr.RecordSearchOutcome(ctx, "id-x", nil)
	row, _ = q.GetIndexer(ctx, "id-x")
	if row.HealthStatus != HealthOK {
		t.Fatalf("health after success = %q, want %q", row.HealthStatus, HealthOK)
	}
	if row.FailureCount != 0 || row.LastError != "" {
		t.Errorf("failure streak not reset: count=%d last_error=%q", row.FailureCount, row.LastError)
	}
	if row.LastSuccessAt == "" {
		t.Error("last_success_at should be stamped")
	}
	rec.waitFor(t, events.TypeIndexerHealthChanged, 2)
}

// A catalog audit must not clear the failing state — the definition
// resolving fine says nothing about live search health.
func TestAuditCatalog_PreservesFailingState(t *testing.T) {
	mgr, q, _ := newTestManager(t)
	createTestIndexer(t, q, "id-x", "SomeIndexer")
	ctx := context.Background()

	boom := errors.New("HTTP 400")
	for i := 0; i < failingThreshold; i++ {
		mgr.RecordSearchOutcome(ctx, "id-x", boom)
	}

	mgr.AuditCatalog(ctx, func(string, string) string { return "someindexer" })

	row, _ := q.GetIndexer(ctx, "id-x")
	if row.HealthStatus != HealthFailing {
		t.Errorf("audit reset health to %q, want %q preserved", row.HealthStatus, HealthFailing)
	}
}

// Search outcomes must not resurrect a definition_missing indexer.
func TestRecordSearchOutcome_DoesNotOverrideDefinitionMissing(t *testing.T) {
	mgr, q, _ := newTestManager(t)
	createTestIndexer(t, q, "id-x", "GoneIndexer")
	ctx := context.Background()

	mgr.AuditCatalog(ctx, func(string, string) string { return "" })
	mgr.RecordSearchOutcome(ctx, "id-x", nil)

	row, _ := q.GetIndexer(ctx, "id-x")
	if row.HealthStatus != HealthDefinitionMissing {
		t.Errorf("health = %q, want %q preserved", row.HealthStatus, HealthDefinitionMissing)
	}
}

// IsLikelyIPBan must match hard Cloudflare bans (fixable by VPN
// rotation) and NOT plain challenges (handled by FlareSolverr).
func TestIsLikelyIPBan(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{`FlareSolverr failed: FlareSolverr error: Error: Error solving the challenge. Cloudflare has blocked this request. Probably your IP is banned for this site`, true},
		{"FlareSolverr error: Cloudflare has blocked this request (Code 1020)", true},
		{"path \"/\": Cloudflare protection detected (HTTP 403) — requires FlareSolverr", false}, // challenge, not ban
		{"HTTP 400", false},
		{"context deadline exceeded", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsLikelyIPBan(tc.in); got != tc.want {
			t.Errorf("IsLikelyIPBan(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// definition_missing indexers must be withheld from the per-service list —
// that is what makes the service's next sync drop its local copy instead
// of failing every search against it.
func TestListIndexersForService_WithholdsDefinitionMissing(t *testing.T) {
	mgr, q, _ := newTestManager(t)
	ctx := context.Background()

	// A service to assign to.
	_, err := q.CreateService(ctx, db.CreateServiceParams{
		ID: "svc-1", Name: "pilot", Type: "tv", ApiUrl: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	createTestIndexer(t, q, "id-tgx", "TorrentGalaxyClone")
	createTestIndexer(t, q, "id-1337x", "1337x")
	for _, id := range []string{"id-tgx", "id-1337x"} {
		if _, err := mgr.Assign(ctx, AssignmentInput{IndexerID: id, ServiceID: "svc-1"}); err != nil {
			t.Fatal(err)
		}
	}

	mgr.AuditCatalog(ctx, func(name, _ string) string {
		if name == "1337x" {
			return "1337x"
		}
		return ""
	})

	rows, err := q.ListIndexersForService(ctx, "svc-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "1337x" {
		names := make([]string, len(rows))
		for i, r := range rows {
			names[i] = r.Name
		}
		t.Errorf("ListIndexersForService = %v, want only 1337x", names)
	}
}
