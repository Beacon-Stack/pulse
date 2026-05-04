package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestAgg() *Aggregator {
	return &Aggregator{HTTP: &http.Client{Timeout: 2 * time.Second}}
}

// fetchHaulStats decodes /api/v1/stats into the dashboard's narrower shape.
func TestFetchHaulStats_Happy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/stats" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"download_speed":   12345,
			"upload_speed":     6789,
			"active_downloads": 3,
			"active_uploads":   1,
			"peers_connected":  20,
		})
	}))
	defer srv.Close()

	a := newTestAgg()
	got := a.fetchHaulStats(context.Background(), srv.URL, "")
	if got == nil {
		t.Fatal("expected non-nil HaulSummary")
	}
	if got.DownloadSpeed != 12345 || got.UploadSpeed != 6789 {
		t.Errorf("speeds = (%d, %d)", got.DownloadSpeed, got.UploadSpeed)
	}
	if got.ActiveDownloads != 3 || got.ActiveUploads != 1 {
		t.Errorf("active = (%d, %d)", got.ActiveDownloads, got.ActiveUploads)
	}
	if got.PeersConnected != 20 {
		t.Errorf("peers = %d", got.PeersConnected)
	}
}

// Empty baseURL → nil result without error or panic.
func TestFetchHaulStats_NoBaseURL(t *testing.T) {
	a := newTestAgg()
	if got := a.fetchHaulStats(context.Background(), "", ""); got != nil {
		t.Errorf("expected nil for empty baseURL, got %v", got)
	}
}

// 5xx → nil (the dashboard surfaces that as "no data").
func TestFetchHaulStats_5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	a := newTestAgg()
	if got := a.fetchHaulStats(context.Background(), srv.URL, ""); got != nil {
		t.Errorf("expected nil on 5xx, got %v", got)
	}
}

// Apikey header is forwarded so registered services can authenticate.
func TestServiceGetJSON_SendsAPIKey(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Api-Key")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	a := newTestAgg()
	var out map[string]any
	_ = a.serviceGetJSON(context.Background(), srv.URL, "/x", "secret123", &out)
	if seen != "secret123" {
		t.Errorf("X-Api-Key = %q, want secret123", seen)
	}
}

// fetchRuntime parses the runtime endpoint shape we just added in haul/pilot/prism.
func TestFetchRuntime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/system/runtime" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"goroutines":         42,
			"heap_alloc_bytes":   1024,
			"heap_in_use_bytes":  2048,
			"heap_objects":       100,
			"num_gc":             5,
			"last_gc_pause_ns":   1234,
			"uptime_seconds":     999,
			"go_version":         "go1.25",
		})
	}))
	defer srv.Close()
	a := newTestAgg()
	got := a.fetchRuntime(context.Background(), srv.URL, "")
	if got == nil {
		t.Fatal("nil runtime")
	}
	if got.Goroutines != 42 || got.HeapAlloc != 1024 || got.NumGC != 5 || got.GoVersion != "go1.25" {
		t.Errorf("runtime fields not decoded correctly: %+v", got)
	}
}

// fetchSpecifics dispatches based on service type/name.
func TestFetchSpecifics_Haul(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/stats":
			_ = json.NewEncoder(w).Encode(map[string]any{"download_speed": 1})
		case strings.HasPrefix(r.URL.Path, "/api/v1/torrents"):
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "t1"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	a := newTestAgg()
	got := a.fetchSpecifics(context.Background(), "Haul", "download-client", srv.URL, "")
	if got == nil {
		t.Fatal("nil specifics for Haul")
	}
	if _, ok := got["stats"]; !ok {
		t.Error("specifics missing stats")
	}
	if _, ok := got["torrents"]; !ok {
		t.Error("specifics missing torrents")
	}
}

func TestFetchSpecifics_Pilot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/queue" {
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	a := newTestAgg()
	got := a.fetchSpecifics(context.Background(), "Pilot", "media-manager", srv.URL, "")
	if got == nil {
		t.Fatal("nil specifics for Pilot")
	}
	if _, ok := got["queue"]; !ok {
		t.Error("specifics missing queue")
	}
}

// Unknown service type → nil specifics (drawer just hides the section).
func TestFetchSpecifics_UnknownType(t *testing.T) {
	a := newTestAgg()
	got := a.fetchSpecifics(context.Background(), "Mystery", "exotic", "http://example.invalid", "")
	if got != nil {
		t.Errorf("expected nil for unknown service type, got %v", got)
	}
}

// Aggregator with no registry returns a clean error rather than panicking.
func TestOverview_NoRegistry(t *testing.T) {
	a := &Aggregator{}
	if _, err := a.Overview(context.Background()); err == nil {
		t.Error("expected error when registry is nil")
	}
}
