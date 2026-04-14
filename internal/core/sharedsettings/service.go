// Package sharedsettings manages the single-row shared_media_handling table.
// These are filesystem/handling settings that apply uniformly across all
// media-manager services (Prism, Pilot). Services pull them via their sync
// loops and overwrite the corresponding fields in their own local settings.
package sharedsettings

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	db "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/events"
)

// Settings is the data shape exposed externally. Matches the DB columns
// 1:1 minus the sentinel `id` (always 1).
type Settings struct {
	ColonReplacement    string `json:"colon_replacement"`
	ImportExtraFiles    bool   `json:"import_extra_files"`
	ExtraFileExtensions string `json:"extra_file_extensions"`
	RenameFiles         bool   `json:"rename_files"`
	UpdatedAt           string `json:"updated_at"`
}

// Service manages the shared settings record.
type Service struct {
	q      db.Querier
	bus    *events.Bus
	logger *slog.Logger
}

// NewService constructs a new shared settings service.
func NewService(q db.Querier, bus *events.Bus, logger *slog.Logger) *Service {
	return &Service{q: q, bus: bus, logger: logger}
}

// Get returns the current shared settings. The row always exists (inserted
// by the migration), so this is a pure read.
func (s *Service) Get(ctx context.Context) (*Settings, error) {
	row, err := s.q.GetSharedMediaHandling(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading shared settings: %w", err)
	}
	return &Settings{
		ColonReplacement:    row.ColonReplacement,
		ImportExtraFiles:    row.ImportExtraFiles,
		ExtraFileExtensions: row.ExtraFileExtensions,
		RenameFiles:         row.RenameFiles,
		UpdatedAt:           row.UpdatedAt,
	}, nil
}

// Update overwrites the settings and publishes an update event.
func (s *Service) Update(ctx context.Context, input Settings) (*Settings, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	row, err := s.q.UpdateSharedMediaHandling(ctx, db.UpdateSharedMediaHandlingParams{
		ColonReplacement:    input.ColonReplacement,
		ImportExtraFiles:    input.ImportExtraFiles,
		ExtraFileExtensions: input.ExtraFileExtensions,
		RenameFiles:         input.RenameFiles,
		UpdatedAt:           now,
	})
	if err != nil {
		return nil, fmt.Errorf("updating shared settings: %w", err)
	}

	s.bus.Publish(ctx, events.Event{
		Type: events.TypeSharedSettingsUpdated,
		Data: map[string]any{
			"colon_replacement": row.ColonReplacement,
		},
	})

	return &Settings{
		ColonReplacement:    row.ColonReplacement,
		ImportExtraFiles:    row.ImportExtraFiles,
		ExtraFileExtensions: row.ExtraFileExtensions,
		RenameFiles:         row.RenameFiles,
		UpdatedAt:           row.UpdatedAt,
	}, nil
}
