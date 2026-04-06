package sdk

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arrsenal/configurarr/internal/api"
	"github.com/arrsenal/configurarr/internal/api/ws"
	appconfig "github.com/arrsenal/configurarr/internal/config"
	cfgstore "github.com/arrsenal/configurarr/internal/core/config"
	"github.com/arrsenal/configurarr/internal/core/indexer"
	"github.com/arrsenal/configurarr/internal/core/registry"
	"github.com/arrsenal/configurarr/internal/core/tag"
	"github.com/arrsenal/configurarr/internal/db"
	dbsqlite "github.com/arrsenal/configurarr/internal/db/generated/sqlite"
	"github.com/arrsenal/configurarr/internal/events"

	_ "modernc.org/sqlite"
)

const testAPIKey = "test-api-key-12345"

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	// In-memory SQLite
	sqlDB, err := sql.Open("sqlite", ":memory:?_foreign_keys=ON")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := db.Migrate(sqlDB); err != nil {
		t.Fatal(err)
	}

	logger := slog.Default()
	bus := events.New(logger)
	queries := dbsqlite.New(sqlDB)

	wsHub := ws.NewHub(logger, []byte(testAPIKey))
	bus.Subscribe(wsHub.HandleEvent)

	router := api.NewRouter(api.RouterConfig{
		Auth:            appconfig.Secret(testAPIKey),
		Logger:          logger,
		StartTime:       time.Now(),
		RegistryService: registry.NewService(queries, bus, logger),
		ConfigStore:     cfgstore.NewStore(queries, bus, logger),
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
		ConfigurarURL:     ts.URL,
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
		ConfigurarURL:     ts.URL,
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

func TestSDKConfigOperations(t *testing.T) {
	ts := setupTestServer(t)

	client, err := New(Config{
		ConfigurarURL:     ts.URL,
		APIKey:            testAPIKey,
		ServiceName:       "config-test",
		ServiceType:       "automation",
		APIURL:            "http://test:1234",
		HeartbeatInterval: time.Hour,
		Logger:            slog.Default(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()

	// Set config via direct API call (SDK is consumer, not producer — use raw HTTP)
	setConfig(t, ts.URL, "quality", "preferred_codec", "x265")
	setConfig(t, ts.URL, "quality", "min_resolution", "1080p")

	// Read config
	entries, err := client.GetConfigNamespace(ctx, "quality")
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries in quality namespace, got %d", len(entries))
	}

	entry, err := client.GetConfig(ctx, "quality", "preferred_codec")
	if err != nil {
		t.Fatalf("get entry: %v", err)
	}
	if entry.Value != "x265" {
		t.Errorf("expected value x265, got %s", entry.Value)
	}

	// Subscribe
	if err := client.Subscribe(ctx, "quality"); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Unsubscribe
	if err := client.Unsubscribe(ctx, "quality"); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
}

func TestSDKDeregister(t *testing.T) {
	ts := setupTestServer(t)

	client, err := New(Config{
		ConfigurarURL:     ts.URL,
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

// Helper: set config via direct HTTP
func setConfig(t *testing.T, baseURL, namespace, key, value string) {
	t.Helper()
	body := fmt.Sprintf(`{"namespace":"%s","key":"%s","value":"%s"}`, namespace, key, value)
	req, err := http.NewRequest("PUT", baseURL+"/api/v1/config", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Api-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("set config: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		t.Fatalf("set config returned %d", resp.StatusCode)
	}
}
