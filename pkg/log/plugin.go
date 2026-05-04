package log

// plugin.go — Plugin is the contract for shipping logs to external
// systems (Loki, Vector, file, syslog, …). Built-in plugins live
// under pulse/pkg/log/plugins/<name>/. Operators choose which to
// enable via env vars (e.g. BEACON_LOG_LOKI_URL).
//
// Beacon explicitly stays unopinionated about which log backend an
// operator runs — the in-app viewer always works (it reads from
// Docker stdout or the ring buffer), and shipping to a central
// system is purely additive. A misbehaving plugin must never break
// the rest of logging — TeeHandler isolates panics + swallows
// errors per-sink for that reason.

import (
	"context"
	"log/slog"
)

// Plugin wraps an slog.Handler with metadata + lifecycle. Build
// plugins by:
//  1. Implementing this interface in pulse/pkg/log/plugins/<name>/.
//  2. Returning a constructor func(context.Context, Config) (Plugin, error).
//  3. Wiring the constructor into the host service's main.go via
//     env-var detection (or making it always-on if it's a default).
//
// The pattern mirrors the existing Pilot plugin layout (downloaders,
// indexers) so the convention is familiar to anyone touching
// Beacon's plugin code.
type Plugin interface {
	// Name returns a stable identifier for logs/diagnostics
	// ("loki", "file", "vector", …).
	Name() string

	// Handler returns the slog.Handler this plugin contributes to
	// the tee fan-out. Called once at startup.
	Handler() slog.Handler

	// Close flushes any pending writes and releases resources. Called
	// on graceful shutdown. Plugins MUST be tolerant of being closed
	// before they've fully drained — losing a handful of buffered
	// records on shutdown is acceptable.
	Close(ctx context.Context) error
}
