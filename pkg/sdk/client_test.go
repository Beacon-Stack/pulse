package sdk

import (
	"context"
	"database/sql"
	"os"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/beacon-stack/pulse/internal/api"
	"github.com/beacon-stack/pulse/internal/api/ws"
	appconfig "github.com/beacon-stack/pulse/internal/config"
	"github.com/beacon-stack/pulse/internal/core/indexer"
	"github.com/beacon-stack/pulse/internal/core/registry"
	"github.com/beacon-stack/pulse/internal/core/tag"
	"github.com/beacon-stack/pulse/internal/db"
	dbgen "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/events"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const testAPIKey = "test-api-key-12345"

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN not set — skipping integration test")
	}

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := db.Migrate(sqlDB); err != nil {
		t.Fatal(err)
	}

	// Clean all tables for test isolation.
	for _, table := range []string{"indexer_tags", "service_tags", "tags", "config_subscriptions", "config_entries", "indexer_assignments", "indexers", "service_capabilities", "services", "filter_presets", "download_clients"} {
		_, _ = sqlDB.Exec("DELETE FROM " + table)
	}

	logger := slog.Default()
	bus := events.New(logger)
	queries := dbgen.New(sqlDB)

	wsHub := ws.NewHub(logger, []byte(testAPIKey))
	bus.Subscribe(wsHub.HandleEvent)

	router := api.NewRouter(api.RouterConfig{
		Auth:            appconfig.Secret(testAPIKey),
		Logger:          logger,
		StartTime:       time.Now(),
		RegistryService: registry.NewService(queries, bus, logger),
		IndexerManager:  indexer.NewManager(queries, bus, logger),
		TagService:      tag.NewService(queries),
		WSHub:           wsHub,
	})

	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)
	return ts
}

func TestSDKRegisterAndDiscover(t *testing.T) {
	ts := setupTestServer(t)

	// Register service A
	clientA, err := New(Config{
		PulseURL:     ts.URL,
		APIKey:            testAPIKey,
		ServiceName:       "test-downloader",
		ServiceType:       "download-client",
		APIURL:            "http://downloader:8080",
		HealthURL:         "http://downloader:8080/health",
		Version:           "1.0.0",
		Capabilities:      []string{"supports_torrent", "supports_categories"},
		HeartbeatInterval: time.Hour, // don't actually heartbeat during test
		Logger:            slog.Default(),
	})
	if err != nil {
		t.Fatalf("register A: %v", err)
	}
	defer clientA.Close()

	if clientA.ServiceID() == "" {
		t.Fatal("expected non-empty service ID")
	}

	// Register service B
	clientB, err := New(Config{
		PulseURL:     ts.URL,
		APIKey:            testAPIKey,
		ServiceName:       "test-luminarr",
		ServiceType:       "media-manager",
		APIURL:            "http://luminarr:8282",
		Version:           "0.1.0",
		Capabilities:      []string{"supports_torrent", "supports_usenet"},
		HeartbeatInterval: time.Hour,
		Logger:            slog.Default(),
	})
	if err != nil {
		t.Fatalf("register B: %v", err)
	}
	defer clientB.Close()

	ctx := context.Background()

	// Discover download clients from B's perspective
	downloaders, err := clientB.DiscoverByType(ctx, "download-client")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(downloaders) != 1 {
		t.Fatalf("expected 1 download client, got %d", len(downloaders))
	}
	if downloaders[0].Name != "test-downloader" {
		t.Errorf("expected name test-downloader, got %s", downloaders[0].Name)
	}

	// Discover by capability
	torrent, err := clientB.DiscoverByCapability(ctx, "supports_torrent")
	if err != nil {
		t.Fatalf("discover by capability: %v", err)
	}
	if len(torrent) != 2 {
		t.Errorf("expected 2 services with supports_torrent, got %d", len(torrent))
	}

	// Discover all
	all, err := clientB.DiscoverAll(ctx)
	if err != nil {
		t.Fatalf("discover all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 total services, got %d", len(all))
	}
}

func TestSDKDeregister(t *testing.T) {
	ts := setupTestServer(t)

	client, err := New(Config{
		PulseURL:     ts.URL,
		APIKey:            testAPIKey,
		ServiceName:       "ephemeral",
		ServiceType:       "automation",
		APIURL:            "http://test:1234",
		HeartbeatInterval: time.Hour,
		Logger:            slog.Default(),
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Should exist
	all, _ := client.DiscoverAll(ctx)
	if len(all) != 1 {
		t.Fatalf("expected 1 service, got %d", len(all))
	}

	// Deregister
	if err := client.Deregister(ctx); err != nil {
		t.Fatalf("deregister: %v", err)
	}

	// Create a new client just for discovery (without registering again)
	raw := &Client{
		cfg:    client.cfg,
		client: &http.Client{Timeout: 5 * time.Second},
		logger: slog.Default(),
	}
	all2, _ := raw.DiscoverAll(ctx)
	if len(all2) != 0 {
		t.Errorf("expected 0 services after deregister, got %d", len(all2))
	}
}

