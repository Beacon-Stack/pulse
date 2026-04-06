package tag

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	dbsqlite "github.com/arrsenal/configurarr/internal/db/generated/sqlite"
)

// TagWithCounts is a tag enriched with usage counts.
type TagWithCounts struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ServiceCount int64  `json:"service_count"`
	IndexerCount int64  `json:"indexer_count"`
}

// Service manages tags.
type Service struct {
	q dbsqlite.Querier
}

// NewService creates a new tag service.
func NewService(q dbsqlite.Querier) *Service {
	return &Service{q: q}
}

// List returns all tags with usage counts.
func (s *Service) List(ctx context.Context) ([]TagWithCounts, error) {
	tags, err := s.q.ListTags(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TagWithCounts, len(tags))
	for i, t := range tags {
		svcCount, _ := s.q.CountServicesForTag(ctx, t.ID)
		idxCount, _ := s.q.CountIndexersForTag(ctx, t.ID)
		out[i] = TagWithCounts{
			ID:           t.ID,
			Name:         t.Name,
			ServiceCount: svcCount,
			IndexerCount: idxCount,
		}
	}
	return out, nil
}

// Create creates a new tag.
func (s *Service) Create(ctx context.Context, name string) (*dbsqlite.Tag, error) {
	row, err := s.q.CreateTag(ctx, dbsqlite.CreateTagParams{
		ID:   uuid.New().String(),
		Name: name,
	})
	if err != nil {
		return nil, fmt.Errorf("tag already exists or DB error: %w", err)
	}
	return &row, nil
}

// Update renames a tag.
func (s *Service) Update(ctx context.Context, id, name string) (*dbsqlite.Tag, error) {
	row, err := s.q.UpdateTag(ctx, dbsqlite.UpdateTagParams{
		Name: name,
		ID:   id,
	})
	if err != nil {
		return nil, fmt.Errorf("tag not found or name conflict: %w", err)
	}
	return &row, nil
}

// Delete removes a tag.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.q.DeleteTag(ctx, id)
}
