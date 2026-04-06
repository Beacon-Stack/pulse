package registry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	dbsqlite "github.com/beacon-media/pulse/internal/db/generated/sqlite"
	"github.com/beacon-media/pulse/internal/events"
)

// ServiceInput is the data required to register or update a service.
type ServiceInput struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	APIURL       string   `json:"api_url"`
	APIKey       string   `json:"api_key"`
	HealthURL    string   `json:"health_url"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	Metadata     string   `json:"metadata"`
}

// ServiceInfo is the enriched view returned from queries.
type ServiceInfo struct {
	dbsqlite.Service
	Capabilities []string `json:"capabilities"`
}

// OnRegisterFunc is called after a service registers. Used for retroactive
// indexer auto-assignment without circular imports.
type OnRegisterFunc func(ctx context.Context, serviceID string)

// Service manages the service registry.
type Service struct {
	q          dbsqlite.Querier
	bus        *events.Bus
	logger     *slog.Logger
	onRegister OnRegisterFunc
}

// NewService creates a new registry service.
func NewService(q dbsqlite.Querier, bus *events.Bus, logger *slog.Logger) *Service {
	return &Service{q: q, bus: bus, logger: logger}
}

// SetOnRegister sets a callback that fires after a new service registers.
func (s *Service) SetOnRegister(fn OnRegisterFunc) {
	s.onRegister = fn
}

// Register adds or updates a service in the registry.
// If a service with the same name+type exists, it updates; otherwise creates.
func (s *Service) Register(ctx context.Context, input ServiceInput) (*ServiceInfo, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	if input.Metadata == "" {
		input.Metadata = "{}"
	}

	// Check if already registered.
	existing, err := s.q.GetServiceByNameAndType(ctx, dbsqlite.GetServiceByNameAndTypeParams{
		Name: input.Name,
		Type: input.Type,
	})
	if err == nil {
		// Update existing registration.
		row, err := s.q.UpdateService(ctx, dbsqlite.UpdateServiceParams{
			Name:     input.Name,
			ApiUrl:   input.APIURL,
			ApiKey:   input.APIKey,
			HealthUrl: input.HealthURL,
			Version:  input.Version,
			Metadata: input.Metadata,
			LastSeen: now,
			ID:       existing.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("updating service: %w", err)
		}

		caps, err := s.syncCapabilities(ctx, row.ID, input.Capabilities)
		if err != nil {
			return nil, err
		}

		s.bus.Publish(ctx, events.Event{
			Type:      events.TypeServiceRegistered,
			ServiceID: row.ID,
			Data:      map[string]any{"name": row.Name, "type": row.Type, "action": "updated"},
		})

		return &ServiceInfo{Service: row, Capabilities: caps}, nil
	}

	// Create new registration.
	id := uuid.New().String()
	row, err := s.q.CreateService(ctx, dbsqlite.CreateServiceParams{
		ID:         id,
		Name:       input.Name,
		Type:       input.Type,
		ApiUrl:     input.APIURL,
		ApiKey:     input.APIKey,
		HealthUrl:  input.HealthURL,
		Version:    input.Version,
		Status:     "online",
		LastSeen:   now,
		Registered: now,
		Metadata:   input.Metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("creating service: %w", err)
	}

	caps, err := s.syncCapabilities(ctx, id, input.Capabilities)
	if err != nil {
		return nil, err
	}

	s.bus.Publish(ctx, events.Event{
		Type:      events.TypeServiceRegistered,
		ServiceID: id,
		Data:      map[string]any{"name": row.Name, "type": row.Type, "action": "created"},
	})

	// Retroactive auto-assignment: assign existing indexers to this new service.
	if s.onRegister != nil {
		go s.onRegister(context.WithoutCancel(ctx), id)
	}

	return &ServiceInfo{Service: row, Capabilities: caps}, nil
}

// Deregister removes a service from the registry.
func (s *Service) Deregister(ctx context.Context, id string) error {
	svc, err := s.q.GetService(ctx, id)
	if err != nil {
		return fmt.Errorf("service not found: %w", err)
	}

	if err := s.q.DeleteCapabilities(ctx, id); err != nil {
		return fmt.Errorf("deleting capabilities: %w", err)
	}
	if err := s.q.DeleteService(ctx, id); err != nil {
		return fmt.Errorf("deleting service: %w", err)
	}

	s.bus.Publish(ctx, events.Event{
		Type:      events.TypeServiceDeregistered,
		ServiceID: id,
		Data:      map[string]any{"name": svc.Name, "type": svc.Type},
	})

	return nil
}

// Get returns a single service with its capabilities.
func (s *Service) Get(ctx context.Context, id string) (*ServiceInfo, error) {
	svc, err := s.q.GetService(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service not found: %w", err)
	}
	caps, err := s.q.ListCapabilities(ctx, id)
	if err != nil {
		return nil, err
	}
	return &ServiceInfo{Service: svc, Capabilities: caps}, nil
}

// List returns all registered services.
func (s *Service) List(ctx context.Context) ([]ServiceInfo, error) {
	rows, err := s.q.ListServices(ctx)
	if err != nil {
		return nil, err
	}
	return s.enrichServices(ctx, rows)
}

// ListByType returns services filtered by type.
func (s *Service) ListByType(ctx context.Context, svcType string) ([]ServiceInfo, error) {
	rows, err := s.q.ListServicesByType(ctx, svcType)
	if err != nil {
		return nil, err
	}
	return s.enrichServices(ctx, rows)
}

// ListByCapability returns services that declare a specific capability.
func (s *Service) ListByCapability(ctx context.Context, capability string) ([]ServiceInfo, error) {
	rows, err := s.q.ListServicesByCapability(ctx, capability)
	if err != nil {
		return nil, err
	}
	return s.enrichServices(ctx, rows)
}

// Heartbeat updates the last_seen timestamp and sets status to online.
func (s *Service) Heartbeat(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	return s.q.UpdateServiceHeartbeat(ctx, dbsqlite.UpdateServiceHeartbeatParams{
		LastSeen: now,
		ID:       id,
	})
}

// UpdateStatus sets the status for a service.
func (s *Service) UpdateStatus(ctx context.Context, id, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	return s.q.UpdateServiceStatus(ctx, dbsqlite.UpdateServiceStatusParams{
		Status:   status,
		LastSeen: now,
		ID:       id,
	})
}

func (s *Service) syncCapabilities(ctx context.Context, serviceID string, capabilities []string) ([]string, error) {
	if err := s.q.DeleteCapabilities(ctx, serviceID); err != nil {
		return nil, fmt.Errorf("clearing capabilities: %w", err)
	}
	for _, cap := range capabilities {
		if err := s.q.AddCapability(ctx, dbsqlite.AddCapabilityParams{
			ID:         uuid.New().String(),
			ServiceID:  serviceID,
			Capability: cap,
		}); err != nil {
			return nil, fmt.Errorf("adding capability %q: %w", cap, err)
		}
	}
	return capabilities, nil
}

func (s *Service) enrichServices(ctx context.Context, rows []dbsqlite.Service) ([]ServiceInfo, error) {
	out := make([]ServiceInfo, len(rows))
	for i, row := range rows {
		caps, err := s.q.ListCapabilities(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		out[i] = ServiceInfo{Service: row, Capabilities: caps}
	}
	return out, nil
}
