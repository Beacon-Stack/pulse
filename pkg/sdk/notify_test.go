package sdk

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
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
	"github.com/beacon-stack/pulse/pkg/notify"

	_ "modernc.org/sqlite"
)

// receiver stands in for Discord/ntfy/Home Assistant: it records events.
type receiver struct {
	mu  sync.Mutex
	got []notify.Event
}

func (r *receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var e notify.Event
	_ = json.NewDecoder(req.Body).Decode(&e)
	r.mu.Lock()
	r.got = append(r.got, e)
	r.mu.Unlock()
}

func (r *receiver) events() []notify.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]notify.Event(nil), r.got...)
}

func pulseWithNotifications(t *testing.T) *httptest.Server {
	t.Helper()
	database, err := db.Open(appconfig.DatabaseConfig{Path: filepath.Join(t.TempDir(), "pulse.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(database.SQL); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := events.New(logger)
	queries := dbgen.New(database.SQL)
	store, err := notify.NewStore(context.Background(), database.SQL)
	if err != nil {
		t.Fatal(err)
	}
	router := api.NewRouter(api.RouterConfig{
		Auth:            appconfig.Secret(testAPIKey),
		Logger:          logger,
		StartTime:       time.Now(),
		RegistryService: registry.NewService(queries, bus, logger),
		IndexerManager:  indexer.NewManager(queries, bus, logger),
		TagService:      tag.NewService(queries),
		WSHub:           ws.NewHub(logger),
		NotifyStore:     store,
		NotifyOutbox:    &notify.Outbox{App: "Pulse", Store: store, Log: logger},
	})
	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)
	return ts
}

func call(t *testing.T, method, url string, body any, out any) int {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, r)
	req.Header.Set("X-Api-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

// An app connected to Pulse hands its events to Pulse, which broadcasts
// them on its own channels; the app's channels stay quiet.
func TestNotifications_AppThroughPulse(t *testing.T) {
	pulse := pulseWithNotifications(t)
	ha := &receiver{}
	haSrv := httptest.NewServer(ha)
	t.Cleanup(haSrv.Close)

	// Set up a channel in Pulse, only for Pilot's imports.
	var created map[string]any
	if code := call(t, http.MethodPost, pulse.URL+"/api/v1/notifications/channels", map[string]any{
		"name": "Home Assistant", "kind": "webhook", "enabled": true,
		"settings": map[string]any{"url": haSrv.URL, "secret": "s3cret"},
		"events":   []string{"imported"}, "apps": []string{"pilot"},
	}, &created); code != http.StatusCreated {
		t.Fatalf("create channel: %d %v", code, created)
	}
	if s := created["settings"].(map[string]any); s["url"] != notify.Masked || s["secret"] != notify.Masked {
		t.Errorf("secrets must come back masked: %v", s)
	}

	// The app: its own database and Pulse connection.
	client, err := New(Config{
		PulseURL: pulse.URL, APIKey: testAPIKey, ServiceName: "pilot", ServiceType: "media-manager",
		APIURL: "http://pilot:8383", HealthURL: "http://pilot:8383/health", Version: "1",
		HeartbeatInterval: time.Hour, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	appDB, _ := sql.Open("sqlite", filepath.Join(t.TempDir(), "pilot.db"))
	t.Cleanup(func() { _ = appDB.Close() })
	appStore, err := notify.NewStore(context.Background(), appDB)
	if err != nil {
		t.Fatal(err)
	}
	local := &receiver{}
	localSrv := httptest.NewServer(local)
	t.Cleanup(localSrv.Close)
	if _, err := appStore.Create(context.Background(), notify.Channel{Name: "App's own", Kind: "webhook", Enabled: true,
		Settings: json.RawMessage(`{"url":"` + localSrv.URL + `"}`)}); err != nil {
		t.Fatal(err)
	}
	outbox := &notify.Outbox{App: "Pilot", Store: appStore, Hub: client.NotifyHub, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	if r := outbox.Route(context.Background()); !r.ViaPulse || r.PulseChannels != 1 {
		t.Fatalf("route %+v", r)
	}
	outbox.Publish(notify.Event{Type: notify.TypeImported, Title: "Imported: Andor S02E03", Level: notify.LevelSuccess})
	outbox.Publish(notify.Event{Type: notify.TypeGrabbed, Title: "Grabbed: Andor S02E04"}) // Pulse's channel doesn't want this

	deadline := time.Now().Add(3 * time.Second)
	for len(ha.events()) < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	got := ha.events()
	if len(got) != 1 || got[0].App != "Pilot" || got[0].Title != "Imported: Andor S02E03" {
		t.Fatalf("Home Assistant got %+v", got)
	}
	if n := len(local.events()); n != 0 {
		t.Errorf("the app's own channel must stay quiet when Pulse handles it; got %d", n)
	}

	var recent []notify.Sent
	call(t, http.MethodGet, pulse.URL+"/api/v1/notifications/recent", nil, &recent)
	if len(recent) == 0 || recent[0].App != "Pilot" || len(recent[0].Results) != 1 || !recent[0].Results[0].OK() {
		t.Errorf("Pulse's recent list %+v", recent)
	}
}

func TestNotifications_PulseAPI(t *testing.T) {
	pulse := pulseWithNotifications(t)

	var cat struct {
		Hub   bool `json:"hub"`
		Kinds []struct {
			ID string `json:"id"`
		} `json:"kinds"`
		Types []notify.TypeInfo     `json:"types"`
		Apps  []struct{ ID string } `json:"apps"`
	}
	call(t, http.MethodGet, pulse.URL+"/api/v1/notifications/catalog", nil, &cat)
	if !cat.Hub || len(cat.Kinds) != 8 || len(cat.Types) == 0 || len(cat.Apps) != 4 {
		t.Errorf("catalog %+v", cat)
	}

	var bad map[string]any
	if code := call(t, http.MethodPost, pulse.URL+"/api/v1/notifications/channels", map[string]any{
		"name": "Phone", "kind": "telegram", "enabled": true, "settings": map[string]any{"bot_token": "x"},
	}, &bad); code != http.StatusBadRequest || !strings.Contains(bad["detail"].(string), "Chat ID is required") {
		t.Errorf("missing setting: %d %v", code, bad)
	}

	if code := call(t, http.MethodPost, pulse.URL+"/api/v1/notifications/send", map[string]any{
		"app": "Pilot", "type": "task_started", "title": "x",
	}, &bad); code != http.StatusBadRequest {
		t.Errorf("internal event types must be refused: %d", code)
	}

	var test struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	call(t, http.MethodPost, pulse.URL+"/api/v1/notifications/test", map[string]any{
		"kind": "webhook", "settings": map[string]any{"url": "http://169.254.169.254/x"},
	}, &test)
	if test.OK || !strings.Contains(test.Error, "not allowed") {
		t.Errorf("test against the metadata address: %+v", test)
	}

	// Pulse has no "send through Pulse" switch of its own. (Unknown paths
	// fall through to the web app's page, so check no JSON API answered.)
	req, _ := http.NewRequest(http.MethodPut, pulse.URL+"/api/v1/notifications/mode", strings.NewReader(`{"mode":"local"}`))
	req.Header.Set("X-Api-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if strings.Contains(resp.Header.Get("Content-Type"), "json") {
		t.Errorf("Pulse should not have a mode endpoint (got %d %s)", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}
