package health

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	dbsqlite "github.com/arrsenal/configurarr/internal/db/generated/sqlite"
	"github.com/arrsenal/configurarr/internal/events"
)

const (
	checkTimeout = 5 * time.Second
	// StaleThreshold is how long since last_seen before a service is considered offline.
	StaleThreshold = 2 * time.Minute
)

// Checker polls registered services for health status.
type Checker struct {
	q      dbsqlite.Querier
	bus    *events.Bus
	client *http.Client
	logger *slog.Logger
}

// NewChecker creates a health checker.
func NewChecker(q dbsqlite.Querier, bus *events.Bus, logger *slog.Logger) *Checker {
	return &Checker{
		q:   q,
		bus: bus,
		client: &http.Client{
			Timeout: checkTimeout,
		},
		logger: logger,
	}
}

// CheckAll polls all services with a health URL and updates their status.
func (c *Checker) CheckAll(ctx context.Context) {
	services, err := c.q.ListServices(ctx)
	if err != nil {
		c.logger.Error("health: failed to list services", "error", err)
		return
	}

	var wg sync.WaitGroup
	for _, svc := range services {
		wg.Add(1)
		go func(s dbsqlite.Service) {
			defer wg.Done()
			c.checkOne(ctx, s)
		}(svc)
	}
	wg.Wait()
}

func (c *Checker) checkOne(ctx context.Context, svc dbsqlite.Service) {
	now := time.Now().UTC()

	// If no health URL, check staleness via last_seen.
	if svc.HealthUrl == "" {
		c.checkStaleness(ctx, svc, now)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, svc.HealthUrl, nil)
	if err != nil {
		c.markStatus(ctx, svc, "offline", now)
		return
	}

	if svc.ApiKey != "" {
		req.Header.Set("X-Api-Key", svc.ApiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.markStatus(ctx, svc, "offline", now)
		return
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		c.markStatus(ctx, svc, "online", now)
	case resp.StatusCode >= 500:
		c.markStatus(ctx, svc, "degraded", now)
	default:
		c.markStatus(ctx, svc, "offline", now)
	}
}

func (c *Checker) checkStaleness(ctx context.Context, svc dbsqlite.Service, now time.Time) {
	if svc.LastSeen == "" {
		return
	}
	lastSeen, err := time.Parse(time.RFC3339, svc.LastSeen)
	if err != nil {
		return
	}
	if now.Sub(lastSeen) > StaleThreshold && svc.Status != "offline" {
		c.markStatus(ctx, svc, "offline", now)
	}
}

func (c *Checker) markStatus(ctx context.Context, svc dbsqlite.Service, status string, now time.Time) {
	oldStatus := svc.Status
	if oldStatus == status {
		return
	}

	if err := c.q.UpdateServiceStatus(ctx, dbsqlite.UpdateServiceStatusParams{
		Status:   status,
		LastSeen: now.Format(time.RFC3339),
		ID:       svc.ID,
	}); err != nil {
		c.logger.Error("health: failed to update status",
			"service", svc.Name, "error", err)
		return
	}

	var eventType events.Type
	switch status {
	case "online":
		eventType = events.TypeServiceOnline
	case "offline":
		eventType = events.TypeServiceOffline
	case "degraded":
		eventType = events.TypeServiceDegraded
	default:
		eventType = events.TypeHealthCheck
	}

	c.bus.Publish(ctx, events.Event{
		Type:      eventType,
		ServiceID: svc.ID,
		Data: map[string]any{
			"name":       svc.Name,
			"old_status": oldStatus,
			"new_status": status,
		},
	})

	c.logger.Info("health: service status changed",
		"service", svc.Name,
		"from", oldStatus,
		"to", status,
	)
}
