package config

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	dbsqlite "github.com/beacon-media/pulse/internal/db/generated/sqlite"
	"github.com/beacon-media/pulse/internal/events"
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
	q      dbsqlite.Querier
	bus    *events.Bus
	logger *slog.Logger
}

// NewStore creates a new config store.
func NewStore(q dbsqlite.Querier, bus *events.Bus, logger *slog.Logger) *Store {
	return &Store{q: q, bus: bus, logger: logger}
}

// Set creates or updates a config entry. Publishes a config_updated event
// so subscribers are notified.
func (s *Store) Set(ctx context.Context, namespace, key, value string) (*Entry, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	row, err := s.q.SetConfigEntry(ctx, dbsqlite.SetConfigEntryParams{
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
	row, err := s.q.GetConfigEntry(ctx, dbsqlite.GetConfigEntryParams{
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

// ListNamespace returns all config entries in a namespace.
func (s *Store) ListNamespace(ctx context.Context, namespace string) ([]Entry, error) {
	rows, err := s.q.ListConfigByNamespace(ctx, namespace)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, len(rows))
	for i, r := range rows {
		out[i] = Entry{Namespace: r.Namespace, Key: r.Key, Value: r.Value, UpdatedAt: r.UpdatedAt}
	}
	return out, nil
}

// ListAll returns every config entry.
func (s *Store) ListAll(ctx context.Context) ([]Entry, error) {
	rows, err := s.q.ListAllConfig(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, len(rows))
	for i, r := range rows {
		out[i] = Entry{Namespace: r.Namespace, Key: r.Key, Value: r.Value, UpdatedAt: r.UpdatedAt}
	}
	return out, nil
}

// ListNamespaces returns all distinct namespace names.
func (s *Store) ListNamespaces(ctx context.Context) ([]string, error) {
	return s.q.ListConfigNamespaces(ctx)
}

// Delete removes a config entry.
func (s *Store) Delete(ctx context.Context, namespace, key string) error {
	if err := s.q.DeleteConfigEntry(ctx, dbsqlite.DeleteConfigEntryParams{
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

// DeleteNamespace removes all entries in a namespace.
func (s *Store) DeleteNamespace(ctx context.Context, namespace string) error {
	return s.q.DeleteConfigNamespace(ctx, namespace)
}

// Subscribe registers a service as interested in a config namespace.
func (s *Store) Subscribe(ctx context.Context, serviceID, namespace string) error {
	return s.q.Subscribe(ctx, dbsqlite.SubscribeParams{
		ID:        uuid.New().String(),
		ServiceID: serviceID,
		Namespace: namespace,
	})
}

// Unsubscribe removes a service's subscription to a namespace.
func (s *Store) Unsubscribe(ctx context.Context, serviceID, namespace string) error {
	return s.q.Unsubscribe(ctx, dbsqlite.UnsubscribeParams{
		ServiceID: serviceID,
		Namespace: namespace,
	})
}

// ListSubscriptions returns the namespaces a service is subscribed to.
func (s *Store) ListSubscriptions(ctx context.Context, serviceID string) ([]string, error) {
	return s.q.ListSubscriptionsByService(ctx, serviceID)
}

// ListSubscribers returns the services subscribed to a namespace.
func (s *Store) ListSubscribers(ctx context.Context, namespace string) ([]dbsqlite.Service, error) {
	return s.q.ListSubscribersByNamespace(ctx, namespace)
}
