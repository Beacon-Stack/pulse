package indexer

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	dbsqlite "github.com/arrsenal/configurarr/internal/db/generated/sqlite"
)

// categoryToCapability maps indexer categories to service capability names.
// An indexer with category "Movies" auto-assigns to services with "content:movies".
var categoryToCapability = map[string]string{
	"Movies": "content:movies",
	"TV":     "content:tv",
	"Audio":  "content:audio",
	"Books":  "content:books",
	"XXX":    "content:xxx",
	"Other":  "content:other",
}

// AutoAssigner handles automatic indexer-to-service assignment based on
// category↔capability matching.
type AutoAssigner struct {
	q      dbsqlite.Querier
	pusher *Pusher
	logger *slog.Logger
}

// NewAutoAssigner creates a new AutoAssigner.
func NewAutoAssigner(q dbsqlite.Querier, pusher *Pusher, logger *slog.Logger) *AutoAssigner {
	return &AutoAssigner{q: q, pusher: pusher, logger: logger}
}

// AssignIndexerToMatchingServices finds all services whose content:* capabilities
// match the indexer's categories and creates assignments. Called when an indexer
// is created.
func (a *AutoAssigner) AssignIndexerToMatchingServices(ctx context.Context, indexerID string, categories []string) {
	capabilities := categoriesToCapabilities(categories)
	if len(capabilities) == 0 {
		return
	}

	// Collect all services that match any of the capabilities.
	matched := map[string]bool{}
	for _, cap := range capabilities {
		services, err := a.q.ListServicesByCapability(ctx, cap)
		if err != nil {
			a.logger.Warn("autoassign: failed to query services by capability",
				"capability", cap, "error", err)
			continue
		}
		for _, svc := range services {
			matched[svc.ID] = true
		}
	}

	if len(matched) == 0 {
		return
	}

	// Check existing assignments to avoid duplicates.
	existing, _ := a.q.ListAssignmentsByIndexer(ctx, indexerID)
	alreadyAssigned := map[string]bool{}
	for _, a := range existing {
		alreadyAssigned[a.ServiceID] = true
	}

	for serviceID := range matched {
		if alreadyAssigned[serviceID] {
			continue
		}

		if _, err := a.q.CreateAssignment(ctx, dbsqlite.CreateAssignmentParams{
			ID:        uuid.New().String(),
			IndexerID: indexerID,
			ServiceID: serviceID,
			Overrides: "{}",
		}); err != nil {
			a.logger.Warn("autoassign: failed to create assignment",
				"indexer_id", indexerID, "service_id", serviceID, "error", err)
			continue
		}

		// Look up service name for logging.
		svc, _ := a.q.GetService(ctx, serviceID)
		svcName := serviceID
		if svc.Name != "" {
			svcName = svc.Name
		}

		a.logger.Info("autoassign: assigned indexer to service",
			"indexer_id", indexerID, "service", svcName)

		// Push-notify the service.
		a.pusher.NotifyServiceAsync(serviceID)
	}
}

// AssignExistingIndexersToService finds all unassigned indexers whose categories
// match the service's content:* capabilities and creates assignments.
// Called when a new service registers.
func (a *AutoAssigner) AssignExistingIndexersToService(ctx context.Context, serviceID string) {
	// Get the service's content capabilities.
	caps, err := a.q.ListCapabilities(ctx, serviceID)
	if err != nil {
		a.logger.Warn("autoassign: failed to list service capabilities", "error", err)
		return
	}

	contentCaps := map[string]bool{}
	for _, cap := range caps {
		if strings.HasPrefix(cap, "content:") {
			contentCaps[cap] = true
		}
	}
	if len(contentCaps) == 0 {
		return
	}

	// Get all indexers.
	indexers, err := a.q.ListIndexers(ctx)
	if err != nil {
		a.logger.Warn("autoassign: failed to list indexers", "error", err)
		return
	}

	// Get existing assignments for this service.
	existingAssignments, _ := a.q.ListAssignmentsByService(ctx, serviceID)
	alreadyAssigned := map[string]bool{}
	for _, a := range existingAssignments {
		alreadyAssigned[a.IndexerID] = true
	}

	assigned := 0
	for _, idx := range indexers {
		if alreadyAssigned[idx.ID] {
			continue
		}

		// Look up the indexer's categories from the catalog.
		cats := catalogCategoriesForIndexer(idx.Name)
		idxCaps := categoriesToCapabilities(cats)

		// Check if any of the indexer's capabilities match the service.
		match := false
		for _, cap := range idxCaps {
			if contentCaps[cap] {
				match = true
				break
			}
		}
		if !match {
			continue
		}

		if _, err := a.q.CreateAssignment(ctx, dbsqlite.CreateAssignmentParams{
			ID:        uuid.New().String(),
			IndexerID: idx.ID,
			ServiceID: serviceID,
			Overrides: "{}",
		}); err != nil {
			continue
		}

		a.logger.Info("autoassign: assigned existing indexer to new service",
			"indexer", idx.Name, "service_id", serviceID)
		assigned++
	}

	if assigned > 0 {
		a.pusher.NotifyServiceAsync(serviceID)
		a.logger.Info("autoassign: retroactive assignment complete",
			"service_id", serviceID, "assigned", assigned)
	}
}

func categoriesToCapabilities(categories []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, cat := range categories {
		cap, ok := categoryToCapability[cat]
		if ok && !seen[cap] {
			seen[cap] = true
			out = append(out, cap)
		}
	}
	return out
}

// catalogCategoriesForIndexer looks up the categories for an indexer by name
// from the built-in catalog. Returns nil if not found (e.g., generic indexers).
func catalogCategoriesForIndexer(name string) []string {
	for _, e := range builtinCatalog {
		if strings.EqualFold(e.Name, name) {
			return e.Categories
		}
	}
	return nil
}
