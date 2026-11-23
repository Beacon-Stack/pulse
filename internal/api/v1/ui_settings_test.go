package v1

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
	_ "modernc.org/sqlite"

	cfgstore "github.com/beacon-stack/pulse/internal/core/config"
	pulsedb "github.com/beacon-stack/pulse/internal/db"
	db "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/events"
)

func newUISettingsAPI(t *testing.T) humatest.TestAPI {
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
	store := cfgstore.NewStore(db.New(sqlDB), events.New(slog.Default()), slog.Default())
	_, api := humatest.New(t)
	RegisterUISettingsRoutes(api, store)
	return api
}

func TestAddIndexerDefaults(t *testing.T) {
	api := newUISettingsAPI(t)
	_ = context.Background()

	// Never set: empty lists, not nulls.
	resp := api.Get("/api/v1/settings/add-indexer-defaults")
	if resp.Code != http.StatusOK || strings.Contains(resp.Body.String(), "null") {
		t.Fatalf("unset defaults = %d %s", resp.Code, resp.Body.String())
	}

	resp = api.Put("/api/v1/settings/add-indexer-defaults", map[string]any{
		"protocols": []string{"torrent"}, "privacies": []string{"public"}, "languages": []string{"en-US"},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("save = %d %s", resp.Code, resp.Body.String())
	}
	got := api.Get("/api/v1/settings/add-indexer-defaults").Body.String()
	for _, want := range []string{`"protocols":["torrent"]`, `"privacies":["public"]`, `"languages":["en-US"]`, `"categories":[]`} {
		if !strings.Contains(got, want) {
			t.Errorf("saved defaults %s missing %s", got, want)
		}
	}

	resp = api.Put("/api/v1/settings/add-indexer-defaults", map[string]any{"privacies": []string{"secret"}})
	if resp.Code != http.StatusBadRequest {
		t.Errorf("unknown privacy accepted: %d", resp.Code)
	}
}
