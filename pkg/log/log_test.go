package log

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// TestNew_StampsServiceField: every emission carries the service
// field automatically — that's the canonical filter for grepping
// across services and Loki's primary label, so a regression here
// is loud.
func TestNew_StampsServiceField(t *testing.T) {
	// Capture stdout via a temp file pointed at the configured Output.
	tmp := filepath.Join(t.TempDir(), "out.log")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer f.Close()

	logger, _ := New(Config{
		Service: "pulse",
		Level:   "info",
		Output:  f,
	})
	logger.Info("hello")
	_ = f.Sync()

	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("read tmp: %v", err)
	}
	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &rec); err != nil {
		t.Fatalf("not valid JSON: %v\nbody=%s", err, data)
	}
	if rec["service"] != "pulse" {
		t.Errorf("expected service=pulse, got %v", rec["service"])
	}
	if rec["msg"] != "hello" {
		t.Errorf("expected msg=hello, got %v", rec["msg"])
	}
}

// TestNew_RingBufferCapturesEverything: sanity that records also
// land in the in-memory buffer that the API endpoint reads from.
func TestNew_RingBufferCapturesEverything(t *testing.T) {
	logger, sys := New(Config{
		Service:    "pulse",
		Level:      "debug",
		Output:     os.Stdout, // ignored — test reads buffer
		BufferSize: 10,
	})
	logger.Debug("d")
	logger.Info("i")
	logger.Warn("w")

	if got := sys.Buffer.Len(); got != 3 {
		t.Errorf("buffer Len = %d, want 3", got)
	}
}

// TestNew_LevelChangesPropagateAtRuntime: the headline runtime-
// flip behavior — the user changes the level via the API and
// debug records that were dropping start flowing immediately.
func TestNew_LevelChangesPropagateAtRuntime(t *testing.T) {
	logger, sys := New(Config{
		Service:    "pulse",
		Level:      "info", // debug suppressed
		Output:     os.Stdout,
		BufferSize: 10,
	})
	logger.Debug("first") // should be dropped
	if got := sys.Buffer.Len(); got != 0 {
		t.Errorf("debug at info-level should be dropped: Len = %d, want 0", got)
	}

	if err := sys.Level.Set("debug"); err != nil {
		t.Fatalf("set debug: %v", err)
	}
	logger.Debug("second") // should land
	if got := sys.Buffer.Len(); got != 1 {
		t.Errorf("after Set(debug), Len = %d, want 1 — runtime level change must propagate to ring buffer", got)
	}
}

// fakePlugin lets us assert Add + Close lifecycle.
type fakePlugin struct {
	closed bool
	h      slog.Handler
}

func (p *fakePlugin) Name() string             { return "fake" }
func (p *fakePlugin) Handler() slog.Handler    { return p.h }
func (p *fakePlugin) Close(_ context.Context) error {
	p.closed = true
	return nil
}

// TestSystem_AddRegistersHandler + TestSystem_CloseClosesAllPlugins
// pin the plugin lifecycle: Add wires the handler in for future
// records, Close calls each plugin's Close in registration order.
func TestSystem_AddRegistersHandler(t *testing.T) {
	logger, sys := New(Config{Service: "pulse", BufferSize: 10})
	rec := &recordingHandler{}
	sys.Add(&fakePlugin{h: rec})
	logger.Info("after-add")
	if rec.count.Load() != 1 {
		t.Errorf("plugin handler missed the record: count=%d", rec.count.Load())
	}
}

func TestSystem_CloseClosesAllPlugins(t *testing.T) {
	_, sys := New(Config{Service: "pulse", BufferSize: 10})
	p1 := &fakePlugin{h: &recordingHandler{}}
	p2 := &fakePlugin{h: &recordingHandler{}}
	sys.Add(p1)
	sys.Add(p2)

	sys.Close(context.Background())
	if !p1.closed || !p2.closed {
		t.Errorf("Close should call every plugin: p1=%v p2=%v", p1.closed, p2.closed)
	}
}
