// Package qualityprofile manages centralized quality profiles that are
// distributed to media-manager services (Prism, Pilot) via their sync loops.
package qualityprofile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	db "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/events"
)

// ErrNotFound is returned when a quality profile does not exist.
var ErrNotFound = errors.New("quality profile not found")

// Input is the data needed to create or update a quality profile.
// The JSON fields (CutoffJSON, QualitiesJSON, UpgradeUntilJSON) are passed
// through verbatim to consumers — Pulse does not interpret their contents.
type Input struct {
	Name                 string  `json:"name"`
	CutoffJSON           string  `json:"cutoff_json"`
	QualitiesJSON        string  `json:"qualities_json"`
	UpgradeAllowed       bool    `json:"upgrade_allowed"`
	UpgradeUntilJSON     *string `json:"upgrade_until_json,omitempty"`
	MinCustomFormatScore int     `json:"min_custom_format_score"`
	UpgradeUntilCFScore  int     `json:"upgrade_until_cf_score"`
}

// Service manages quality profile records.
type Service struct {
	q      db.Querier
	bus    *events.Bus
	logger *slog.Logger
}

// NewService creates a new quality profile service.
func NewService(q db.Querier, bus *events.Bus, logger *slog.Logger) *Service {
	return &Service{q: q, bus: bus, logger: logger}
}

// Create adds a new quality profile.
func (s *Service) Create(ctx context.Context, input Input) (*db.QualityProfile, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if input.CutoffJSON == "" {
		input.CutoffJSON = "{}"
	}
	if input.QualitiesJSON == "" {
		input.QualitiesJSON = "[]"
	}

	row, err := s.q.CreateQualityProfile(ctx, db.CreateQualityProfileParams{
		ID:                   uuid.New().String(),
		Name:                 input.Name,
		CutoffJson:           input.CutoffJSON,
		QualitiesJson:        input.QualitiesJSON,
		UpgradeAllowed:       input.UpgradeAllowed,
		UpgradeUntilJson:     nullStringFromPtr(input.UpgradeUntilJSON),
		MinCustomFormatScore: int32(input.MinCustomFormatScore),
		UpgradeUntilCfScore:  int32(input.UpgradeUntilCFScore),
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err != nil {
		return nil, fmt.Errorf("creating quality profile: %w", err)
	}

	s.bus.Publish(ctx, events.Event{
		Type: events.TypeQualityProfileCreated,
		Data: map[string]any{"id": row.ID, "name": row.Name},
	})

	return &row, nil
}

// Get returns a single quality profile by ID.
func (s *Service) Get(ctx context.Context, id string) (*db.QualityProfile, error) {
	row, err := s.q.GetQualityProfile(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting quality profile: %w", err)
	}
	return &row, nil
}

// List returns all quality profiles, sorted by name.
func (s *Service) List(ctx context.Context) ([]db.QualityProfile, error) {
	rows, err := s.q.ListQualityProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing quality profiles: %w", err)
	}
	return rows, nil
}

// Update modifies an existing quality profile.
func (s *Service) Update(ctx context.Context, id string, input Input) (*db.QualityProfile, error) {
	if input.CutoffJSON == "" {
		input.CutoffJSON = "{}"
	}
	if input.QualitiesJSON == "" {
		input.QualitiesJSON = "[]"
	}

	row, err := s.q.UpdateQualityProfile(ctx, db.UpdateQualityProfileParams{
		Name:                 input.Name,
		CutoffJson:           input.CutoffJSON,
		QualitiesJson:        input.QualitiesJSON,
		UpgradeAllowed:       input.UpgradeAllowed,
		UpgradeUntilJson:     nullStringFromPtr(input.UpgradeUntilJSON),
		MinCustomFormatScore: int32(input.MinCustomFormatScore),
		UpgradeUntilCfScore:  int32(input.UpgradeUntilCFScore),
		UpdatedAt:            time.Now().UTC().Format(time.RFC3339),
		ID:                   id,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("updating quality profile: %w", err)
	}

	s.bus.Publish(ctx, events.Event{
		Type: events.TypeQualityProfileUpdated,
		Data: map[string]any{"id": row.ID, "name": row.Name},
	})

	return &row, nil
}

// Delete removes a quality profile. Pulse does not have foreign-key references
// to quality profiles (services hold those), so deletion always succeeds at the
// Pulse level. Services will remove their local copies on the next sync.
func (s *Service) Delete(ctx context.Context, id string) error {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	if err := s.q.DeleteQualityProfile(ctx, id); err != nil {
		return fmt.Errorf("deleting quality profile: %w", err)
	}

	s.bus.Publish(ctx, events.Event{
		Type: events.TypeQualityProfileDeleted,
		Data: map[string]any{"id": id, "name": existing.Name},
	})

	return nil
}

// nullStringFromPtr converts a *string to sql.NullString.
func nullStringFromPtr(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}
