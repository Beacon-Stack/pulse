package indexer

import (
	"context"
	"fmt"
	"strings"
	"time"

	db "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/events"
)

// Indexer health states.
//   - unknown/ok/definition_missing are maintained by AuditCatalog
//     (definition-resolution dimension, boot + daily refresh).
//   - failing is maintained by RecordSearchOutcome (live search
//     dimension): N consecutive search failures flip an indexer to
//     failing; one success flips it back to ok. Unlike
//     definition_missing, failing indexers are still synced to services
//     — they may recover on their own.
const (
	HealthUnknown           = "unknown"
	HealthOK                = "ok"
	HealthDefinitionMissing = "definition_missing"
	HealthFailing           = "failing"
)

// failingThreshold is how many consecutive search failures flip an
// indexer to the failing state. Mirrors Prowlarr's failure-tracking
// approach: one transient error is noise, three in a row is a signal.
const failingThreshold = 3

// ipBanSignatures are substrings (matched case-insensitively against an
// indexer's last_error) that indicate the egress IP itself is banned by
// Cloudflare — the failure mode a VPN IP rotation actually fixes. The
// plain challenge signal ("Cloudflare protection detected") is
// deliberately absent: a challenge is normal and FlareSolverr handles
// it; only a hard ban warrants rotating.
var ipBanSignatures = []string{
	"cloudflare has blocked",
	"ip is banned",
	"code 1020",
}

// IsLikelyIPBan reports whether a search error message looks like a
// Cloudflare IP ban (vs an ordinary failure). Used by the API to offer
// the "Rotate VPN IP" action in context.
func IsLikelyIPBan(lastError string) bool {
	if lastError == "" {
		return false
	}
	lower := strings.ToLower(lastError)
	for _, sig := range ipBanSignatures {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

// RecordSearchOutcome updates an indexer's search-health counters after a
// proxied search. searchErr == nil records a success (resets the failure
// streak and, if the indexer was failing, restores it to ok).
func (m *Manager) RecordSearchOutcome(ctx context.Context, indexerID string, searchErr error) {
	row, err := m.q.GetIndexer(ctx, indexerID)
	if err != nil {
		m.logger.Warn("search health: indexer lookup failed", "id", indexerID, "error", err)
		return
	}
	// The definition dimension owns this state; search outcomes must not
	// resurrect an indexer whose definition is gone.
	if row.HealthStatus == HealthDefinitionMissing {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	status := row.HealthStatus
	message := row.HealthMessage
	failureCount := row.FailureCount
	var lastError string
	lastSuccess := row.LastSuccessAt

	if searchErr == nil {
		failureCount = 0
		lastError = ""
		lastSuccess = now
		status = HealthOK
		message = ""
	} else {
		failureCount++
		lastError = truncate(searchErr.Error(), 500) // reuses tester.go's helper
		if failureCount >= failingThreshold {
			status = HealthFailing
			message = fmt.Sprintf("%d consecutive search failures; last: %s", failureCount, lastError)
		}
	}

	if err := m.q.SetIndexerSearchHealth(ctx, db.SetIndexerSearchHealthParams{
		HealthStatus:    status,
		HealthMessage:   message,
		HealthCheckedAt: now,
		FailureCount:    failureCount,
		LastError:       lastError,
		LastSuccessAt:   lastSuccess,
		ID:              row.ID,
	}); err != nil {
		m.logger.Warn("search health: failed to record outcome",
			"indexer", row.Name, "error", err)
		return
	}

	if status != row.HealthStatus {
		if status == HealthFailing {
			m.logger.Warn("search health: indexer marked failing",
				"indexer", row.Name, "failures", failureCount, "last_error", lastError)
		} else {
			m.logger.Info("search health: indexer recovered", "indexer", row.Name)
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
	}
}

// AuditCatalog cross-checks every configured indexer against the loaded
// Prowlarr definition catalog and records the result in the indexer's
// health columns. It runs after each definition load (boot and the daily
// refresh), using the same resolver the torznab proxy uses at search time
// so the audit can never disagree with what a search would hit.
//
// Without this, an indexer whose upstream definition was removed (a site
// shuts down and Prowlarr deletes its definition) stays configured and
// assigned, and the first place it fails is every search.
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
		} else if row.HealthStatus == HealthFailing {
			// The definition resolving fine says nothing about live search
			// health — leave the failing state to RecordSearchOutcome.
			status, message = HealthFailing, row.HealthMessage
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
