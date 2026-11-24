package gluetun

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/beacon-stack/pulse/internal/events"
)

// rotateClient is the slice of Client the Rotator needs — an interface so
// tests can drive rotation against a fake without a control server.
type rotateClient interface {
	PublicIP(ctx context.Context) (string, error)
	SetOpenVPNStatus(ctx context.Context, status string) error
}

// Rotation pacing. maxAttempts exists because the provider can reconnect
// to the very same server and hand back the same IP;
// minInterval keeps a click-happy user (or a future automation bug) from
// thrashing the tunnel — every rotation pauses torrent traffic and
// invalidates every FlareSolverr session.
const (
	rotateMinInterval  = 5 * time.Minute
	rotateMaxAttempts  = 3
	rotatePollInterval = 2 * time.Second
	rotatePollBudget   = 60 * time.Second
)

// RotateResult reports what a rotation accomplished.
type RotateResult struct {
	OldIP    string `json:"old_ip"`
	NewIP    string `json:"new_ip"`
	Attempts int    `json:"attempts"`
	Changed  bool   `json:"changed"`
}

// ErrRotateTooSoon is returned when a rotation is requested inside the
// cool-down window. Wrapped with the remaining wait.
var ErrRotateTooSoon = fmt.Errorf("rotated too recently")

// Rotator cycles the VPN tunnel until the provider assigns a different
// exit IP, then clears IP-bound scraper state (FlareSolverr sessions,
// result cache) and kicks off a session pre-warm.
type Rotator struct {
	client            rotateClient
	clearScraperState func()                    // nil-able
	prewarm           func(ctx context.Context) // nil-able
	bus               *events.Bus               // nil-able
	logger            *slog.Logger

	// Pacing — set from the package consts; overridable in tests.
	minInterval  time.Duration
	pollInterval time.Duration
	pollBudget   time.Duration

	mu         sync.Mutex
	rotating   bool
	lastRotate time.Time

	// Region changes; nil/empty until EnableRegions.
	regionClient regionClient
	catalog      RegionCatalog
	prefs        RegionPreference
}

// begin claims the tunnel for one reconnect (rotation or region change),
// refusing while another runs or inside the cool-down. Pair with end.
func (r *Rotator) begin() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rotating {
		return fmt.Errorf("a VPN reconnect is already in progress")
	}
	if wait := r.minInterval - time.Since(r.lastRotate); !r.lastRotate.IsZero() && wait > 0 {
		return fmt.Errorf("%w: try again in %s", ErrRotateTooSoon, wait.Truncate(time.Second))
	}
	r.rotating = true
	return nil
}

func (r *Rotator) end() {
	r.mu.Lock()
	r.rotating = false
	r.lastRotate = time.Now()
	r.mu.Unlock()
}

// NewRotator wires a Rotator. client must be non-nil (pass nil to the
// caller's RouterConfig instead when gluetun is disabled).
func NewRotator(client rotateClient, clearScraperState func(), prewarm func(ctx context.Context), bus *events.Bus, logger *slog.Logger) *Rotator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Rotator{
		client:            client,
		clearScraperState: clearScraperState,
		prewarm:           prewarm,
		bus:               bus,
		logger:            logger,
		minInterval:       rotateMinInterval,
		pollInterval:      rotatePollInterval,
		pollBudget:        rotatePollBudget,
	}
}

// Rotate cycles the tunnel until the public IP changes, up to
// rotateMaxAttempts cycles. A same-IP outcome after all attempts is NOT
// an error — the tunnel is healthy, the provider just pinned the server;
// Changed=false lets the caller word that honestly. IP-bound scraper
// state is cleared after any successful reconnect, changed or not (the
// tunnel bounce invalidated the sessions either way).
func (r *Rotator) Rotate(ctx context.Context) (RotateResult, error) {
	if err := r.begin(); err != nil {
		return RotateResult{}, err
	}
	defer r.end()

	oldIP, err := r.client.PublicIP(ctx)
	if err != nil {
		return RotateResult{}, fmt.Errorf("reading current public IP: %w", err)
	}

	result := RotateResult{OldIP: oldIP}
	for attempt := 1; attempt <= rotateMaxAttempts; attempt++ {
		result.Attempts = attempt
		r.logger.Info("vpn rotate: cycling tunnel", "attempt", attempt, "old_ip", oldIP)

		if err := r.client.SetOpenVPNStatus(ctx, "stopped"); err != nil {
			return result, fmt.Errorf("stopping tunnel: %w", err)
		}
		if err := r.client.SetOpenVPNStatus(ctx, "running"); err != nil {
			return result, fmt.Errorf("restarting tunnel: %w", err)
		}

		newIP, err := r.waitForIP(ctx)
		if err != nil {
			return result, err
		}
		result.NewIP = newIP
		if newIP != oldIP {
			result.Changed = true
			break
		}
	}

	// Sessions are IP-bound and the tunnel bounced regardless of outcome.
	r.afterReconnect(ctx, map[string]any{
		"old_ip":   result.OldIP,
		"new_ip":   result.NewIP,
		"attempts": result.Attempts,
		"changed":  result.Changed,
	})
	r.logger.Info("vpn rotate: done",
		"old_ip", result.OldIP, "new_ip", result.NewIP,
		"attempts", result.Attempts, "changed", result.Changed)
	return result, nil
}

// waitForIP polls the public-IP endpoint until the reconnected tunnel
// reports a non-empty IP.
func (r *Rotator) waitForIP(ctx context.Context) (string, error) {
	deadline := time.Now().Add(r.pollBudget)
	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("tunnel did not report a public IP within %s", r.pollBudget)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(r.pollInterval):
		}
		ip, err := r.client.PublicIP(ctx)
		if err == nil && ip != "" {
			return ip, nil
		}
	}
}
