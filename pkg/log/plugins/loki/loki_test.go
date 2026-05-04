package loki

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLoki_PushesBatchedRecords: verify the plugin POSTs a Loki-
// shaped payload with our service+level labels and the records as
// values. Uses a httptest server playing the role of Loki.
func TestLoki_PushesBatchedRecords(t *testing.T) {
	var pushes atomic.Int32
	var mu sync.Mutex
	var lastBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/push" {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		lastBody = body
		mu.Unlock()
		pushes.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p, err := New(Config{
		Service:          "pulse",
		URL:              srv.URL,
		MaxBatchSize:     5,
		MaxBatchInterval: 50 * time.Millisecond,
		HTTPClient:       srv.Client(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	logger := slog.New(p.Handler())
	for i := 0; i < 5; i++ {
		logger.Info("event", "i", i)
	}

	// Wait briefly for the batcher to flush.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && pushes.Load() == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	if pushes.Load() == 0 {
		t.Fatal("Loki server got 0 pushes — batcher did not flush within 1s")
	}

	mu.Lock()
	body := lastBody
	mu.Unlock()

	var payload struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
			Values [][2]string       `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("payload not JSON: %v body=%s", err, body)
	}
	if len(payload.Streams) == 0 {
		t.Fatal("payload had no streams")
	}
	stream := payload.Streams[0]
	if stream.Stream["service"] != "pulse" {
		t.Errorf("stream.service = %q, want pulse", stream.Stream["service"])
	}
	if stream.Stream["level"] != "INFO" {
		t.Errorf("stream.level = %q, want INFO", stream.Stream["level"])
	}
	if len(stream.Values) == 0 {
		t.Error("no values in stream")
	}

	_ = p.Close(context.Background())
}

// TestLoki_RequiresURLAndService: a misconfigured plugin should
// fail loudly at startup so the operator notices, rather than
// silently dropping records into the void.
func TestLoki_RequiresURLAndService(t *testing.T) {
	if _, err := New(Config{Service: "pulse"}); err == nil {
		t.Error("expected error when URL is empty")
	}
	if _, err := New(Config{URL: "http://x"}); err == nil {
		t.Error("expected error when Service is empty")
	}
}

// TestLoki_DropsWhenChannelFull: when Loki is unreachable, the
// batcher's buffer fills and excess records are dropped (via
// non-blocking send). Verify Handle never blocks.
func TestLoki_DropsWhenChannelFull(t *testing.T) {
	// Server takes longer than the client Timeout — simulates a
	// slow/hung Loki. Using a finite sleep (not `select {}`) so
	// httptest.Server.Close() can drain connections at test end.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, err := New(Config{
		Service:          "pulse",
		URL:              srv.URL,
		MaxBatchSize:     2,
		MaxBatchInterval: time.Hour, // never flushes naturally
		Timeout:          50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close(context.Background())

	logger := slog.New(p.Handler())

	// 10000 records into a 4-slot buffer. If Handle blocked, this
	// would never finish.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			logger.Info("noise", "i", i)
		}
		close(done)
	}()
	select {
	case <-done:
		// expected — Handle is non-blocking
	case <-time.After(2 * time.Second):
		t.Fatal("Handle blocked under backpressure — should drop instead")
	}
}
