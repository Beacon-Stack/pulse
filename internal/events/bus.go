package events

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Type identifies what happened.
type Type string

const (
	TypeServiceRegistered   Type = "service_registered"
	TypeServiceDeregistered Type = "service_deregistered"
	TypeServiceOnline       Type = "service_online"
	TypeServiceOffline      Type = "service_offline"
	TypeServiceDegraded     Type = "service_degraded"
	TypeConfigUpdated       Type = "config_updated"
	TypeConfigDeleted       Type = "config_deleted"
	TypeIndexerCreated      Type = "indexer_created"
	TypeIndexerUpdated      Type = "indexer_updated"
	TypeIndexerDeleted      Type = "indexer_deleted"
	TypeIndexerAssigned     Type = "indexer_assigned"
	TypeIndexerUnassigned   Type = "indexer_unassigned"
	TypeHealthCheck         Type = "health_check"
	TypeQualityProfileCreated Type = "quality_profile_created"
	TypeQualityProfileUpdated Type = "quality_profile_updated"
	TypeQualityProfileDeleted Type = "quality_profile_deleted"
	TypeSharedSettingsUpdated Type = "shared_settings_updated"
)

// Event carries the context of something that happened.
type Event struct {
	Type      Type           `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	ServiceID string         `json:"service_id,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// Handler is a function that receives events.
type Handler func(ctx context.Context, e Event)

// Bus is a simple in-process publish/subscribe event bus.
// Publish is non-blocking — each handler runs in its own goroutine.
type Bus struct {
	mu       sync.RWMutex
	handlers []Handler
	logger   *slog.Logger
}

// New creates a new Bus.
func New(logger *slog.Logger) *Bus {
	return &Bus{logger: logger}
}

// Subscribe registers a handler to receive all future events.
func (b *Bus) Subscribe(h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, h)
}

// Publish sends an event to all registered handlers asynchronously.
func (b *Bus) Publish(ctx context.Context, e Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}

	handlerCtx := context.WithoutCancel(ctx)

	b.mu.RLock()
	handlers := make([]Handler, len(b.handlers))
	copy(handlers, b.handlers)
	b.mu.RUnlock()

	for _, h := range handlers {
		go func(fn Handler) {
			defer func() {
				if r := recover(); r != nil {
					b.logger.Error("event handler panicked",
						"event_type", e.Type,
						"panic", r,
					)
				}
			}()
			fn(handlerCtx, e)
		}(h)
	}
}
