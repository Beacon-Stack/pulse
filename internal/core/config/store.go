package config

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	db "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/events"
)

// Entry represents a config entry with its metadata.
type Entry struct {
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	UpdatedAt string `json:"updated_at"`
}

// Store manages the shared configuration key-value store.
type Store struct {
	q      db.Querier
	bus    *events.Bus
	logger *slog.Logger
}

// NewStore creates a new config store.
func NewStore(q db.Querier, bus *events.Bus, logger *slog.Logger) *Store {
	return &Store{q: q, bus: bus, logger: logger}
}

// Set creates or updates a config entry. Publishes a config_updated event
// so subscribers are notified.
func (s *Store) Set(ctx context.Context, namespace, key, value string) (*Entry, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	row, err := s.q.SetConfigEntry(ctx, db.SetConfigEntryParams{
		ID:        uuid.New().String(),
		Namespace: namespace,
		Key:       key,
		Value:     value,
		UpdatedAt: now,
	})
	if err != nil {
		return nil, fmt.Errorf("setting config entry: %w", err)
	}

	s.bus.Publish(ctx, events.Event{
		Type: events.TypeConfigUpdated,
		Data: map[string]any{
			"namespace": namespace,
			"key":       key,
		},
	})

	return &Entry{
		Namespace: row.Namespace,
		Key:       row.Key,
		Value:     row.Value,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

// Get retrieves a single config entry.
func (s *Store) Get(ctx context.Context, namespace, key string) (*Entry, error) {
	row, err := s.q.GetConfigEntry(ctx, db.GetConfigEntryParams{
		Namespace: namespace,
		Key:       key,
	})
	if err != nil {
		return nil, fmt.Errorf("config entry not found: %w", err)
	}
	return &Entry{
		Namespace: row.Namespace,
		Key:       row.Key,
		Value:     row.Value,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

// Delete removes a config entry.
func (s *Store) Delete(ctx context.Context, namespace, key string) error {
	if err := s.q.DeleteConfigEntry(ctx, db.DeleteConfigEntryParams{
		Namespace: namespace,
		Key:       key,
	}); err != nil {
		return fmt.Errorf("deleting config entry: %w", err)
	}
	s.bus.Publish(ctx, events.Event{
		Type: events.TypeConfigDeleted,
		Data: map[string]any{"namespace": namespace, "key": key},
	})
	return nil
}

// GetAPIKey returns the persisted Pulse API key, or empty string if one
// has never been stored. Typed wrapper around Get so the config package's
// EnsureAPIKey helper can consume this store through a small interface.
func (s *Store) GetAPIKey(ctx context.Context) (string, error) {
	entry, err := s.Get(ctx, "auth", "api_key")
	if err != nil {
		// GetConfigEntry returns "not found" as an error — swallow that and
		// return the empty-string sentinel. Other errors bubble up.
		return "", nil
	}
	return entry.Value, nil
}

// SetAPIKey persists a new Pulse API key. Typed wrapper around Set.
func (s *Store) SetAPIKey(ctx context.Context, value string) error {
	_, err := s.Set(ctx, "auth", "api_key", value)
	return err
}

// Subscribe registers a service as interested in a config namespace.
func (s *Store) Subscribe(ctx context.Context, serviceID, namespace string) error {
	return s.q.Subscribe(ctx, db.SubscribeParams{
		ID:        uuid.New().String(),
		ServiceID: serviceID,
		Namespace: namespace,
	})
}

// Unsubscribe removes a service's subscription to a namespace.
func (s *Store) Unsubscribe(ctx context.Context, serviceID, namespace string) error {
	return s.q.Unsubscribe(ctx, db.UnsubscribeParams{
		ServiceID: serviceID,
		Namespace: namespace,
	})
}
