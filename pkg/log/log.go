// Package log is the shared logging foundation for the Beacon
// ecosystem. Every Beacon service (Pulse, Pilot, Prism, Haul, …)
// uses this package so logs across the stack share field names,
// rendering, level controls, and external-sink wiring.
//
// Goals:
//
//   - **Story-telling stdout**: structured JSON to Docker, with
//     consistent field names so a user grepping logs sees verb +
//     object + outcome on every line.
//   - **In-app viewer**: every service exposes /api/v1/system/logs
//     (and /api/v1/system/log-level) so a user can search, filter
//     by level, and bump verbosity at runtime — no restart.
//   - **Pluggable sinks**: ship to Loki, Vector, files, syslog, etc.
//     via the Plugin interface. Beacon stays unopinionated about
//     which backend you run.
//   - **Always works**: a broken plugin doesn't break stdout or the
//     in-app viewer. A misconfigured env var doesn't fail startup.
//
// Quickstart for a service:
//
//	logger, system := log.New(log.Config{
//	    Service: "pulse",
//	    Level:   "info",
//	})
//	defer system.Close(ctx)
//
//	// Plugins are added through `system.Add(plugin)`. They show
//	// up in subsequent log calls without re-creating the logger.
//	if url := os.Getenv("BEACON_LOG_LOKI_URL"); url != "" {
//	    p, err := loki.New(loki.Config{URL: url, Service: "pulse"})
//	    if err == nil { system.Add(p) }
//	}
//
//	// Mount the runtime endpoints onto the service's Huma API.
//	log.RegisterRoutes(api, system)
package log

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// Config configures a logger for one service. Service is required —
// it shows up in every log line as the "service" field so cross-
// service grepping works without inferring from the container name.
type Config struct {
	// Service identifies the emitting service ("pulse", "pilot", …).
	// Stamped on every record. Required.
	Service string

	// Level is the initial level: "debug", "info", "warn", "error".
	// Unknown values fall back to "info" rather than failing — bad
	// env vars shouldn't block startup. Override at runtime via
	// PUT /api/v1/system/log-level.
	Level string

	// Format is "json" (default) or "text". JSON is what Docker logs
	// + central log servers expect; text is human-readable for local
	// `go run` development.
	Format string

	// BufferSize is the in-memory ring buffer capacity used by the
	// /api/v1/system/logs endpoint when Docker stdout isn't
	// reachable. Default 1000 entries. Set to 0 to disable the
	// buffer entirely (Docker stdout becomes mandatory for the UI).
	BufferSize int

	// Output is where stdout-formatted records go. Defaults to
	// os.Stdout. Override in tests.
	Output *os.File
}

// System is the runtime handle to a service's logging stack. Holds
// the level knob, ring buffer, plugin list, and the underlying tee
// handler. Each service gets one from log.New and passes it to
// RegisterRoutes for the HTTP layer.
type System struct {
	Service string
	Level   *LevelKnob
	Buffer  *RingBuffer
	tee     *TeeHandler
	plugins []Plugin
}

// New constructs the service's logger and returns it alongside the
// System handle. Callers typically:
//
//	logger, system := log.New(cfg)
//	slog.SetDefault(logger)
//	defer system.Close(context.Background())
//
// SetDefault is intentional — Beacon services have lots of code
// paths that grab slog.Default() implicitly (e.g. the SDK), and we
// want all of those to see the configured handler too.
func New(cfg Config) (*slog.Logger, *System) {
	if cfg.Service == "" {
		cfg.Service = "unknown"
	}
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	level := NewLevelKnob(cfg.Level)
	buf := NewRingBuffer(cfg.BufferSize)

	stdout := newStdoutHandler(cfg.Output, cfg.Format, level)
	tee := newTeeHandlerWithLevel(buf, level, stdout)

	logger := slog.New(tee).With(slog.String("service", cfg.Service))

	return logger, &System{
		Service: cfg.Service,
		Level:   level,
		Buffer:  buf,
		tee:     tee,
	}
}

// Add registers a plugin's handler with the tee fan-out. Plugins
// added after New start receiving subsequent records — there's no
// replay of past entries. Safe to call concurrently with logging.
func (s *System) Add(p Plugin) {
	if p == nil || p.Handler() == nil {
		return
	}
	s.tee.AddSink(p.Handler())
	s.plugins = append(s.plugins, p)
}

// Close shuts down all plugins. Idempotent; errors from individual
// plugins are logged at debug level (so the operator can see them
// without spam in normal shutdown). Best-effort.
func (s *System) Close(ctx context.Context) {
	for _, p := range s.plugins {
		if err := p.Close(ctx); err != nil {
			slog.Default().Debug("log plugin close failed", "plugin", p.Name(), "error", err)
		}
	}
}

// newStdoutHandler picks JSON vs text and binds the level knob so
// runtime level changes propagate. cfg.Format == "text" picks the
// human-readable handler; everything else (default) is JSON.
func newStdoutHandler(out *os.File, format string, level *LevelKnob) slog.Handler {
	opts := &slog.HandlerOptions{Level: level}
	if strings.ToLower(format) == "text" {
		return slog.NewTextHandler(out, opts)
	}
	return slog.NewJSONHandler(out, opts)
}
