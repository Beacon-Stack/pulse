package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	db "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/events"
)

// Input is the data required to create or update an indexer.
type Input struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Enabled  bool   `json:"enabled"`
	Priority int    `json:"priority"`
	URL      string `json:"url"`
	APIKey   string `json:"api_key"`
	Settings string `json:"settings"`
}

// AssignmentInput specifies how to assign an indexer to a service.
type AssignmentInput struct {
	IndexerID string `json:"indexer_id"`
	ServiceID string `json:"service_id"`
	Overrides string `json:"overrides"`
}

// IndexerInfo is the enriched view with assignments.
type IndexerInfo struct {
	db.Indexer
	AssignedServices []string `json:"assigned_services,omitempty"`
}

// Manager handles centralized indexer management.
type Manager struct {
	q            db.Querier
	bus          *events.Bus
	pusher       *Pusher
	autoAssigner *AutoAssigner
	logger       *slog.Logger
}

// NewManager creates a new indexer manager.
func NewManager(q db.Querier, bus *events.Bus, logger *slog.Logger) *Manager {
	pusher := NewPusher(q, logger)
	return &Manager{
		q:            q,
		bus:          bus,
		pusher:       pusher,
		autoAssigner: NewAutoAssigner(q, pusher, logger),
		logger:       logger,
	}
}

// AutoAssigner returns the auto-assigner for use by other packages
// (e.g., service registry for retroactive assignment on registration).
func (m *Manager) AutoAssigner() *AutoAssigner {
	return m.autoAssigner
}

// Create adds a new indexer.
func (m *Manager) Create(ctx context.Context, input Input) (*db.Indexer, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if input.Settings == "" {
		input.Settings = "{}"
	}
	if input.Kind == "" {
		input.Kind = "torznab"
	}

	row, err := m.q.CreateIndexer(ctx, db.CreateIndexerParams{
		ID:        uuid.New().String(),
		Name:      input.Name,
		Kind:      input.Kind,
		Enabled:   input.Enabled,
		Priority:  int32(input.Priority),
		Url:       input.URL,
		ApiKey:    input.APIKey,
		Settings:  input.Settings,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return nil, fmt.Errorf("creating indexer: %w", err)
	}

	m.bus.Publish(ctx, events.Event{
		Type: events.TypeIndexerCreated,
		Data: map[string]any{"id": row.ID, "name": row.Name, "kind": row.Kind},
	})

	// Auto-assign to services based on category↔capability matching.
	cats := catalogCategoriesForIndexer(row.Name)
	if len(cats) > 0 {
		go m.autoAssigner.AssignIndexerToMatchingServices(context.WithoutCancel(ctx), row.ID, cats)
	}

	return &row, nil
}

// Get returns a single indexer.
func (m *Manager) Get(ctx context.Context, id string) (*db.Indexer, error) {
	row, err := m.q.GetIndexer(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("indexer not found: %w", err)
	}
	return &row, nil
}

// List returns all indexers.
func (m *Manager) List(ctx context.Context) ([]db.Indexer, error) {
	return m.q.ListIndexers(ctx)
}

// ListEnabled returns only enabled indexers.
func (m *Manager) ListEnabled(ctx context.Context) ([]db.Indexer, error) {
	return m.q.ListEnabledIndexers(ctx)
}

// Update modifies an existing indexer.
func (m *Manager) Update(ctx context.Context, id string, input Input) (*db.Indexer, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if input.Settings == "" {
		input.Settings = "{}"
	}

	row, err := m.q.UpdateIndexer(ctx, db.UpdateIndexerParams{
		Name:      input.Name,
		Kind:      input.Kind,
		Enabled:   input.Enabled,
		Priority:  int32(input.Priority),
		Url:       input.URL,
		ApiKey:    input.APIKey,
		Settings:  input.Settings,
		UpdatedAt: now,
		ID:        id,
	})
	if err != nil {
		return nil, fmt.Errorf("updating indexer: %w", err)
	}

	m.bus.Publish(ctx, events.Event{
		Type: events.TypeIndexerUpdated,
		Data: map[string]any{"id": row.ID, "name": row.Name},
	})

	return &row, nil
}

// Delete removes an indexer and its assignments, and notifies all
// affected services so they can remove the indexer locally.
func (m *Manager) Delete(ctx context.Context, id string) error {
	idx, err := m.q.GetIndexer(ctx, id)
	if err != nil {
		return fmt.Errorf("indexer not found: %w", err)
	}

	// Collect assigned services BEFORE deleting assignments.
	assignments, _ := m.q.ListAssignmentsByIndexer(ctx, id)

	if err := m.q.DeleteAssignmentsByIndexer(ctx, id); err != nil {
		return fmt.Errorf("deleting assignments: %w", err)
	}
	if err := m.q.DeleteIndexer(ctx, id); err != nil {
		return fmt.Errorf("deleting indexer: %w", err)
	}

	m.bus.Publish(ctx, events.Event{
		Type: events.TypeIndexerDeleted,
		Data: map[string]any{"id": id, "name": idx.Name},
	})

	// Notify all previously-assigned services to re-sync.
	seen := map[string]bool{}
	for _, a := range assignments {
		if !seen[a.ServiceID] {
			seen[a.ServiceID] = true
			m.pusher.NotifyServiceAsync(a.ServiceID)
		}
	}

	return nil
}

// Assign links an indexer to a service.
func (m *Manager) Assign(ctx context.Context, input AssignmentInput) (*db.IndexerAssignment, error) {
	if input.Overrides == "" {
		input.Overrides = "{}"
	}

	row, err := m.q.CreateAssignment(ctx, db.CreateAssignmentParams{
		ID:        uuid.New().String(),
		IndexerID: input.IndexerID,
		ServiceID: input.ServiceID,
		Overrides: input.Overrides,
	})
	if err != nil {
		return nil, fmt.Errorf("assigning indexer: %w", err)
	}

	m.bus.Publish(ctx, events.Event{
		Type:      events.TypeIndexerAssigned,
		ServiceID: input.ServiceID,
		Data:      map[string]any{"indexer_id": input.IndexerID, "service_id": input.ServiceID},
	})

	// Push-notify the service to sync immediately.
	m.pusher.NotifyServiceAsync(input.ServiceID)

	return &row, nil
}

// Unassign removes an indexer-service link.
func (m *Manager) Unassign(ctx context.Context, indexerID, serviceID string) error {
	if err := m.q.DeleteAssignment(ctx, db.DeleteAssignmentParams{
		IndexerID: indexerID,
		ServiceID: serviceID,
	}); err != nil {
		return fmt.Errorf("unassigning indexer: %w", err)
	}

	m.bus.Publish(ctx, events.Event{
		Type:      events.TypeIndexerUnassigned,
		ServiceID: serviceID,
		Data:      map[string]any{"indexer_id": indexerID, "service_id": serviceID},
	})

	// Push-notify the service to sync immediately.
	m.pusher.NotifyServiceAsync(serviceID)

	return nil
}

// ListForService returns indexers assigned to a specific service.
func (m *Manager) ListForService(ctx context.Context, serviceID string) ([]db.Indexer, error) {
	return m.q.ListIndexersForService(ctx, serviceID)
}

// ListAssignments returns all assignments for an indexer.
func (m *Manager) ListAssignments(ctx context.Context, indexerID string) ([]db.IndexerAssignment, error) {
	return m.q.ListAssignmentsByIndexer(ctx, indexerID)
}
