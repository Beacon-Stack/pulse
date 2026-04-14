package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	db "github.com/beacon-stack/pulse/internal/db/generated"
)

// Pusher notifies services when their indexer assignments change.
// It POSTs to each affected service's sync webhook endpoint.
type Pusher struct {
	q      db.Querier
	client *http.Client
	logger *slog.Logger
}

// NewPusher creates a new Pusher.
func NewPusher(q db.Querier, logger *slog.Logger) *Pusher {
	return &Pusher{
		q:      q,
		client: &http.Client{Timeout: 5 * time.Second},
		logger: logger,
	}
}

// NotifyService sends a sync trigger to a service's Pulse webhook.
// The endpoint is: POST {service.api_url}/api/v1/hooks/pulse/sync
func (p *Pusher) NotifyService(ctx context.Context, serviceID string) {
	svc, err := p.q.GetService(ctx, serviceID)
	if err != nil {
		p.logger.Warn("pusher: service not found", "service_id", serviceID, "error", err)
		return
	}

	if svc.ApiUrl == "" {
		return
	}

	syncURL := svc.ApiUrl + "/api/v1/hooks/pulse/sync"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, syncURL, nil)
	if err != nil {
		p.logger.Warn("pusher: failed to create request", "url", syncURL, "error", err)
		return
	}
	// Use the service's own API key if available.
	if svc.ApiKey != "" {
		req.Header.Set("X-Api-Key", svc.ApiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		p.logger.Debug("pusher: sync notification failed (service may not support it)",
			"service", svc.Name, "url", syncURL, "error", err)
		return
	}
	resp.Body.Close()

	p.logger.Info("pusher: notified service to sync indexers",
		"service", svc.Name,
		"status", resp.StatusCode,
	)
}

// NotifyAllAssigned notifies all services that have the given indexer assigned.
func (p *Pusher) NotifyAllAssigned(ctx context.Context, indexerID string) {
	assignments, err := p.q.ListAssignmentsByIndexer(ctx, indexerID)
	if err != nil {
		p.logger.Warn("pusher: failed to list assignments", "indexer_id", indexerID, "error", err)
		return
	}

	seen := map[string]bool{}
	for _, a := range assignments {
		if seen[a.ServiceID] {
			continue
		}
		seen[a.ServiceID] = true
		go p.NotifyService(ctx, a.ServiceID)
	}
}

// NotifyServiceAsync is a fire-and-forget version of NotifyService.
func (p *Pusher) NotifyServiceAsync(serviceID string) {
	go p.NotifyService(context.Background(), serviceID)
}

func init() {
	// Ensure the interface is satisfied at compile time.
	_ = fmt.Sprintf
}
