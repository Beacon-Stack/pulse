package events

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestBusPublishSubscribe(t *testing.T) {
	bus := New(slog.Default())
	var count atomic.Int32

	bus.Subscribe(func(_ context.Context, e Event) {
		count.Add(1)
	})
	bus.Subscribe(func(_ context.Context, e Event) {
		count.Add(1)
	})

	bus.Publish(context.Background(), Event{Type: TypeServiceRegistered})

	// Handlers run in goroutines — give them a moment.
	time.Sleep(50 * time.Millisecond)

	if got := count.Load(); got != 2 {
		t.Errorf("expected 2 handler calls, got %d", got)
	}
}

func TestBusPublishSetsTimestamp(t *testing.T) {
	bus := New(slog.Default())
	received := make(chan Event, 1)

	bus.Subscribe(func(_ context.Context, e Event) {
		received <- e
	})

	bus.Publish(context.Background(), Event{Type: TypeConfigUpdated})

	select {
	case e := <-received:
		if e.Timestamp.IsZero() {
			t.Error("expected non-zero timestamp")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBusPanicRecovery(t *testing.T) {
	bus := New(slog.Default())
	var count atomic.Int32

	// Panicking handler
	bus.Subscribe(func(_ context.Context, _ Event) {
		panic("boom")
	})

	// Normal handler should still run
	bus.Subscribe(func(_ context.Context, _ Event) {
		count.Add(1)
	})

	bus.Publish(context.Background(), Event{Type: TypeHealthCheck})
	time.Sleep(50 * time.Millisecond)

	if got := count.Load(); got != 1 {
		t.Errorf("expected second handler to still run, got %d calls", got)
	}
}
