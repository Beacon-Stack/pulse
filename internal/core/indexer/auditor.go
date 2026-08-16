package indexer

import (
	"context"
	"fmt"
	"time"

	db "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/events"
)

// Indexer health states maintained by AuditCatalog.
const (
	HealthUnknown           = "unknown"
	HealthOK                = "ok"
	HealthDefinitionMissing = "definition_missing"
)

// AuditCatalog cross-checks every configured indexer against the loaded
// Prowlarr definition catalog and records the result in the indexer's
// health columns. It runs after each definition load (boot and the daily
// refresh), using the same resolver the torznab proxy uses at search time
// so the audit can never disagree with what a search would hit.
//
// Without this, an indexer whose upstream definition was removed (the
// 2026-08-16 TorrentGalaxyClone incident — TorrentGalaxy shut down and
// Prowlarr deleted the definition) stays configured and assigned forever,
// and the first place it fails is every user search.
//
// A transition into or out of definition_missing changes what
// ListIndexersForService returns, so assigned services are push-notified
// to re-sync on those transitions only.
func (m *Manager) AuditCatalog(ctx context.Context, resolve func(name, url string) string) {
	rows, err := m.q.ListIndexers(ctx)
	if err != nil {
		m.logger.Warn("catalog audit: failed to list indexers", "error", err)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var missing int
	for _, row := range rows {
		status, message := HealthOK, ""
		if resolve(row.Name, row.Url) == "" {
			status = HealthDefinitionMissing
			message = fmt.Sprintf("No Prowlarr definition found for %q — it may have been removed upstream", row.Name)
			missing++
			m.logger.Warn("catalog audit: indexer definition missing",
				"indexer", row.Name, "id", row.ID)
		}

		if err := m.q.SetIndexerHealth(ctx, db.SetIndexerHealthParams{
			HealthStatus:    status,
			HealthMessage:   message,
			HealthCheckedAt: now,
			ID:              row.ID,
		}); err != nil {
			m.logger.Warn("catalog audit: failed to record indexer health",
				"indexer", row.Name, "error", err)
			continue
		}

		if row.HealthStatus == status {
			continue
		}
		m.bus.Publish(ctx, events.Event{
			Type: events.TypeIndexerHealthChanged,
			Data: map[string]any{
				"id":             row.ID,
				"name":           row.Name,
				"health_status":  status,
				"health_message": message,
			},
		})
		// Only definition_missing transitions change the per-service
		// indexer list, so only those warrant a push re-sync.
		if status == HealthDefinitionMissing || row.HealthStatus == HealthDefinitionMissing {
			m.pusher.NotifyAllAssigned(ctx, row.ID)
		}
	}

	m.logger.Info("catalog audit complete", "indexers", len(rows), "definition_missing", missing)
}
