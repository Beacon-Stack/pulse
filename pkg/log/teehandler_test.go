package log

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
)

// recordingHandler captures every record it sees. Tests use it to
// assert the tee fan-out reaches every sink.
type recordingHandler struct {
	count atomic.Int32
	level slog.Level
	fail  bool
	panic bool
}

func (h *recordingHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	return lvl >= h.level
}
func (h *recordingHandler) Handle(_ context.Context, _ slog.Record) error {
	h.count.Add(1)
	if h.panic {
		panic("simulated sink panic")
	}
	if h.fail {
		return errors.New("simulated sink error")
	}
	return nil
}
func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(_ string) slog.Handler      { return h }

func TestTeeHandler_FanOutsToAllSinks(t *testing.T) {
	rb := NewRingBuffer(10)
	a := &recordingHandler{}
	b := &recordingHandler{}
	tee := NewTeeHandler(rb, a, b)

	logger := slog.New(tee)
	logger.Info("hello", "n", 1)
	logger.Warn("watch out", "x", "y")

	if got := a.count.Load(); got != 2 {
		t.Errorf("sink A saw %d records, want 2", got)
	}
	if got := b.count.Load(); got != 2 {
		t.Errorf("sink B saw %d records, want 2", got)
	}
	if rb.Len() != 2 {
		t.Errorf("ring buffer Len = %d, want 2", rb.Len())
	}
}

func TestTeeHandler_PanickingSinkDoesNotKillOthers(t *testing.T) {
	rb := NewRingBuffer(10)
	bad := &recordingHandler{panic: true}
	good := &recordingHandler{}
	tee := NewTeeHandler(rb, bad, good)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("tee leaked panic: %v", r)
		}
	}()

	slog.New(tee).Info("test")

	if got := good.count.Load(); got != 1 {
		t.Errorf("good sink saw %d records, want 1 — panic in bad sink should not stop dispatch", got)
	}
}

func TestTeeHandler_AddSinkAppliesToFutureRecords(t *testing.T) {
	rb := NewRingBuffer(10)
	a := &recordingHandler{}
	tee := NewTeeHandler(rb, a)

	logger := slog.New(tee)
	logger.Info("first")
	if a.count.Load() != 1 {
		t.Fatalf("a got %d, want 1", a.count.Load())
	}

	b := &recordingHandler{}
	tee.AddSink(b)

	logger.Info("second")
	if a.count.Load() != 2 || b.count.Load() != 1 {
		t.Errorf("after AddSink: a=%d (want 2), b=%d (want 1) — added sink must see new records", a.count.Load(), b.count.Load())
	}
}

func TestTeeHandler_RingBufferCapturesGroupedAttrs(t *testing.T) {
	rb := NewRingBuffer(10)
	tee := NewTeeHandler(rb)

	logger := slog.New(tee).With("service", "pulse").WithGroup("dns").With("host", "8.8.8.8")
	logger.Info("resolved")

	entries := rb.Entries()
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if v, ok := e.Fields["service"]; !ok || v != "pulse" {
		t.Errorf("service field missing or wrong: %v", e.Fields)
	}
	if v, ok := e.Fields["dns.host"]; !ok || v != "8.8.8.8" {
		t.Errorf("expected grouped key 'dns.host' with value '8.8.8.8', got fields=%v", e.Fields)
	}
}
