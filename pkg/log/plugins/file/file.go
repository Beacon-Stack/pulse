// Package file is a simple log-to-disk plugin with size-based
// rotation. Useful for operators who want a persistent record of
// logs separate from Docker's own log files (which rotate on
// container restart and aren't easy to grep).
//
// Design: small, sync, lossy on crash. We don't reach for an
// external rotator like lumberjack to keep the dep tree light;
// rotation is a bounded slot system: when the active file exceeds
// MaxBytes, it's renamed to <path>.1 and a new active file opens.
// MaxFiles controls how many .1, .2, … to keep before deleting.
//
// This plugin writes JSON lines (one record per line), the same
// format as the stdout handler. So you can `cat /logs/pulse.log
// | jq` and it Just Works.
//
// Operator config:
//
//	BEACON_LOG_FILE_PATH      — file path (e.g. /config/logs/pulse.log)
//	BEACON_LOG_FILE_MAX_BYTES — rotate at this size (default 10MB)
//	BEACON_LOG_FILE_MAX_FILES — keep this many rotated files (default 5)
package file

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Config controls the file plugin.
type Config struct {
	Path     string
	MaxBytes int64 // default 10 MB
	MaxFiles int   // default 5
}

// Plugin implements log.Plugin.
type Plugin struct {
	cfg Config
	h   *handler
}

// New opens the file (creating parent dirs if needed) and returns a
// started plugin. Returns an error if the path is empty or the
// parent dir can't be created.
func New(cfg Config) (*Plugin, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("file: Path required")
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 10 << 20 // 10 MB
	}
	if cfg.MaxFiles <= 0 {
		cfg.MaxFiles = 5
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err != nil {
		return nil, fmt.Errorf("file: mkdir parent: %w", err)
	}
	f, err := os.OpenFile(cfg.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("file: open: %w", err)
	}
	return &Plugin{cfg: cfg, h: &handler{cfg: cfg, f: f}}, nil
}

// Name returns "file".
func (p *Plugin) Name() string { return "file" }

// Handler returns the slog.Handler that writes to the file.
func (p *Plugin) Handler() slog.Handler { return p.h }

// Close flushes pending writes and closes the file. Idempotent.
func (p *Plugin) Close(_ context.Context) error {
	p.h.mu.Lock()
	defer p.h.mu.Unlock()
	if p.h.f == nil {
		return nil
	}
	err := p.h.f.Close()
	p.h.f = nil
	return err
}

type handler struct {
	cfg    Config
	mu     sync.Mutex
	f      *os.File
	groups []string
	bound  []slog.Attr
}

func (h *handler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	out := map[string]any{
		"time":  r.Time.UTC().Format(time.RFC3339Nano),
		"level": r.Level.String(),
		"msg":   r.Message,
	}
	for _, a := range h.bound {
		out[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		key := a.Key
		for i := len(h.groups) - 1; i >= 0; i-- {
			key = h.groups[i] + "." + key
		}
		out[key] = a.Value.Any()
		return true
	})

	line, err := json.Marshal(out)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.f == nil {
		return nil
	}
	if _, err := h.f.Write(line); err != nil {
		return err
	}

	// Cheap rotation check: stat the active file. Doing this on
	// every write is fine — stat is microsecond-fast and we're
	// already in a locked section.
	if info, err := h.f.Stat(); err == nil && info.Size() >= h.cfg.MaxBytes {
		_ = h.rotate()
	}
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &handler{
		cfg:    h.cfg,
		f:      h.f,
		groups: h.groups,
		bound:  append(append([]slog.Attr{}, h.bound...), attrs...),
	}
}

func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{
		cfg:    h.cfg,
		f:      h.f,
		groups: append(append([]string{}, h.groups...), name),
		bound:  h.bound,
	}
}

// rotate slides .N → .(N+1), drops the oldest, and renames the
// active file to .1. Caller must hold h.mu. Best-effort: failures
// during rotation log at debug and the active file stays open so
// log writes keep working.
func (h *handler) rotate() error {
	if err := h.f.Close(); err != nil {
		// Reopen-and-continue rather than fail hard.
		h.f, _ = os.OpenFile(h.cfg.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		return err
	}

	// Drop the oldest .N (where N == MaxFiles).
	oldest := fmt.Sprintf("%s.%d", h.cfg.Path, h.cfg.MaxFiles)
	_ = os.Remove(oldest)

	// Slide .(N-1) → .N, .(N-2) → .(N-1), …
	for i := h.cfg.MaxFiles - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", h.cfg.Path, i)
		to := fmt.Sprintf("%s.%d", h.cfg.Path, i+1)
		_ = os.Rename(from, to)
	}

	// Active file → .1
	_ = os.Rename(h.cfg.Path, h.cfg.Path+".1")

	// Open a fresh active file.
	f, err := os.OpenFile(h.cfg.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	h.f = f
	return nil
}
