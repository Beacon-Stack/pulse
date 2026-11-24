package sdk

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/beacon-stack/pulse/internal/api"
	appconfig "github.com/beacon-stack/pulse/internal/config"
	"github.com/beacon-stack/pulse/internal/core/downloadclient"
	"github.com/beacon-stack/pulse/internal/core/registry"
	"github.com/beacon-stack/pulse/internal/db"
	dbgen "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/events"
)

// An app (which calls Pulse with Pulse's key) is handed what it needs to
// reach a download client: the stored password, and for a client that is
// itself registered with Pulse (Haul) the key it registered. A browser on
// the local network, let in without a key, gets neither, and doesn't get
// the apps' keys from the service list either.
func TestSecretsOnlyForAPIKeyCallers(t *testing.T) {
	database, err := db.Open(appconfig.DatabaseConfig{Path: filepath.Join(t.TempDir(), "pulse.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database.SQL); err != nil {
		t.Fatal(err)
	}
	logger := slog.Default()
	bus := events.New(logger)
	queries := dbgen.New(database.SQL)
	ts := httptest.NewServer(api.NewRouter(api.RouterConfig{
		Auth:                  appconfig.Secret(testAPIKey),
		Logger:                logger,
		StartTime:             time.Now(),
		RegistryService:       registry.NewService(queries, bus, logger),
		DownloadClientService: downloadclient.NewService(queries, bus, logger),
	}))
	t.Cleanup(ts.Close)
	ctx := context.Background()

	// Haul registers itself, with its own key.
	haul, err := New(Config{
		PulseURL: ts.URL, APIKey: testAPIKey,
		ServiceName: "haul", ServiceType: "download-client",
		APIURL: "http://vpn:8484", ServiceAPIKey: "hauls-own-key",
		HeartbeatInterval: time.Hour, Logger: logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { haul.Close() })

	// The owner adds two download clients in Pulse.
	post := func(body string) {
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/download-clients", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Api-Key", testAPIKey)
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusCreated {
			t.Fatalf("create download client: %v %v", err, resp)
		}
		resp.Body.Close()
	}
	post(`{"name":"haul","kind":"haul","host":"vpn","port":8484}`)
	post(`{"name":"qbit","kind":"qbittorrent","host":"qbit","port":8080,"username":"admin","password":"hunter2hunter2"}`)

	// What an app gets.
	app, err := New(Config{
		PulseURL: ts.URL, APIKey: testAPIKey,
		ServiceName: "pilot", ServiceType: "media-manager", APIURL: "http://pilot:8383",
		HeartbeatInterval: time.Hour, Logger: logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	clients, err := app.MyDownloadClients(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]DownloadClient{}
	for _, c := range clients {
		got[c.Name] = c
	}
	if got["haul"].APIKey != "hauls-own-key" {
		t.Errorf("the app should get Haul's registered key, got %q", got["haul"].APIKey)
	}
	if got["qbit"].Password != "hunter2hunter2" || got["qbit"].APIKey != "" {
		t.Errorf("the app should get qBittorrent's password and no key: %+v", got["qbit"])
	}

	// What a browser on the local network gets (no key).
	browser := func(path string) string {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d", path, resp.StatusCode)
		}
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	for _, path := range []string{"/api/v1/download-clients", "/api/v1/services"} {
		body := browser(path)
		for _, secret := range []string{"hauls-own-key", "hunter2hunter2"} {
			if strings.Contains(body, secret) {
				t.Errorf("%s gave a browser the secret %q", path, secret)
			}
		}
		var parsed []map[string]any
		if err := json.Unmarshal([]byte(body), &parsed); err != nil || len(parsed) == 0 {
			t.Errorf("%s: unexpected answer %s", path, body)
		}
	}
}
