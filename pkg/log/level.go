package log

// level.go — runtime-mutable log level. Wraps slog.LevelVar (which
// is the slog stdlib's atomic level holder) with a friendly string
// API and JSON-marshalling helpers so the /api/v1/system/log-level
// endpoint stays simple.
//
// Why this matters: a user troubleshooting an issue should be able
// to flip from info → debug live, run the failing operation, and
// flip back without restarting the container. With LevelVar wired
// into the slog handler, every sink (stdout, ring buffer, plugins)
// honours the change immediately.

import (
	"fmt"
	"log/slog"
	"strings"
)

// LevelKnob holds the runtime-mutable level. It IS a *slog.LevelVar
// (embedded so it satisfies slog's Leveler interface directly) plus
// string-friendly helpers for the HTTP layer.
type LevelKnob struct {
	*slog.LevelVar
}

// NewLevelKnob initialises the knob at the given level string. Falls
// back to "info" on unknown input — never errors at construction.
func NewLevelKnob(level string) *LevelKnob {
	v := &slog.LevelVar{}
	v.Set(parseLevel(level))
	return &LevelKnob{LevelVar: v}
}

// Set parses level (debug | info | warn | error, case-insensitive)
// and updates the knob. Returns an error if the input is unknown
// rather than silently coercing — callers in the HTTP layer return
// 400 on unknown levels so a typo doesn't get accepted.
func (k *LevelKnob) Set(level string) error {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		k.LevelVar.Set(slog.LevelDebug)
	case "info":
		k.LevelVar.Set(slog.LevelInfo)
	case "warn", "warning":
		k.LevelVar.Set(slog.LevelWarn)
	case "error", "err":
		k.LevelVar.Set(slog.LevelError)
	default:
		return fmt.Errorf("unknown log level %q (want debug | info | warn | error)", level)
	}
	return nil
}

// String returns the current level as a lowercase keyword suitable
// for the API response.
func (k *LevelKnob) String() string {
	switch k.LevelVar.Level() {
	case slog.LevelDebug:
		return "debug"
	case slog.LevelInfo:
		return "info"
	case slog.LevelWarn:
		return "warn"
	case slog.LevelError:
		return "error"
	default:
		// future-proofing: if someone adds a custom level beyond the
		// four standards (slog supports arbitrary ints), surface the
		// numeric value rather than crashing.
		return fmt.Sprintf("level(%d)", int(k.LevelVar.Level()))
	}
}

// parseLevel is the construction-time parser. Unknown input falls
// back to info because we don't want to fail Boot just because the
// env var is misspelled — the operator should see a normal startup
// log they can search.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "err":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
