package log

// teehandler.go — fan-out slog.Handler.
//
// Three subtleties pinned by tests:
//
//  1. AddSink must reach derivatives. Solved by sharing the sink
//     list via *sinkRegistry.
//  2. The level filter must short-circuit cleanly. We hold a
//     LevelKnob reference (nil = always-enabled fallback).
//  3. With/WithGroup composition must follow slog semantics: an
//     attr added BEFORE a group is NOT scoped into that group; an
//     attr added AFTER a group IS. We track the call chain as an
//     ordered list of ops (`g`roup or `a`ttrs) and replay it on
//     each inner sink + the ring buffer.
//
// All sinks see every record. Sink panics + errors are isolated.

import (
	"context"
	"log/slog"
	"sync"
)

type sinkRegistry struct {
	mu    sync.RWMutex
	items []slog.Handler
}

func (r *sinkRegistry) snapshot() []slog.Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]slog.Handler, len(r.items))
	copy(out, r.items)
	return out
}

func (r *sinkRegistry) add(s slog.Handler) {
	r.mu.Lock()
	r.items = append(r.items, s)
	r.mu.Unlock()
}

// op is one step in the With/WithGroup chain. Captured in order so
// `deriveSink` can replay the user's expressed structure on each
// inner sink. `kind` is 'a' (attrs) or 'g' (group).
type op struct {
	kind  byte
	attrs []slog.Attr
	group string
}

// TeeHandler dispatches records to N sinks plus the ring buffer.
type TeeHandler struct {
	sinks *sinkRegistry
	buf   *RingBuffer
	level slog.Leveler
	chain []op // ordered With*/WithGroup* operations from the user
}

// NewTeeHandler creates a tee with one or more initial sinks.
func NewTeeHandler(buf *RingBuffer, sinks ...slog.Handler) *TeeHandler {
	return newTeeHandler(buf, nil, sinks)
}

// newTeeHandlerWithLevel binds a runtime LevelKnob.
func newTeeHandlerWithLevel(buf *RingBuffer, level slog.Leveler, sinks ...slog.Handler) *TeeHandler {
	return newTeeHandler(buf, level, sinks)
}

func newTeeHandler(buf *RingBuffer, level slog.Leveler, sinks []slog.Handler) *TeeHandler {
	return &TeeHandler{
		sinks: &sinkRegistry{items: sinks},
		buf:   buf,
		level: level,
	}
}

// AddSink adds another handler to the fan-out. Visible to every
// derivative because the underlying registry is shared.
func (h *TeeHandler) AddSink(s slog.Handler) {
	h.sinks.add(s)
}

// Enabled honors the LevelKnob first; if no knob, asks the sinks;
// if neither answers, accepts iff a ring buffer is attached.
func (h *TeeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if h.level != nil {
		return level >= h.level.Level()
	}
	sinks := h.sinks.snapshot()
	if len(sinks) == 0 {
		return h.buf != nil
	}
	for _, s := range sinks {
		if s.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle dispatches the record to every sink + the ring buffer.
func (h *TeeHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.buf != nil {
		h.buf.Add(h.recordToEntry(r))
	}
	for _, s := range h.sinks.snapshot() {
		derived := h.deriveSink(s)
		func(d slog.Handler) {
			defer func() { _ = recover() }()
			_ = d.Handle(ctx, r)
		}(derived)
	}
	return nil
}

// deriveSink replays the chain on the raw sink so its formatter
// sees the same With/WithGroup nesting the user expressed.
func (h *TeeHandler) deriveSink(s slog.Handler) slog.Handler {
	for _, o := range h.chain {
		switch o.kind {
		case 'g':
			s = s.WithGroup(o.group)
		case 'a':
			s = s.WithAttrs(o.attrs)
		}
	}
	return s
}

// WithAttrs appends an 'a' op to the chain.
func (h *TeeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	chain := make([]op, len(h.chain)+1)
	copy(chain, h.chain)
	chain[len(h.chain)] = op{kind: 'a', attrs: attrs}
	return &TeeHandler{
		sinks: h.sinks,
		buf:   h.buf,
		level: h.level,
		chain: chain,
	}
}

// WithGroup appends a 'g' op to the chain.
func (h *TeeHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	chain := make([]op, len(h.chain)+1)
	copy(chain, h.chain)
	chain[len(h.chain)] = op{kind: 'g', group: name}
	return &TeeHandler{
		sinks: h.sinks,
		buf:   h.buf,
		level: h.level,
		chain: chain,
	}
}

// recordToEntry replays the chain to flatten attrs + groups for the
// ring-buffer Entry. Tracks the group prefix as the chain advances
// so an attr bound BEFORE a group is unprefixed and an attr bound
// AFTER is scoped into the group.
func (h *TeeHandler) recordToEntry(r slog.Record) Entry {
	fields := make(map[string]any, r.NumAttrs())
	var prefix []string

	for _, o := range h.chain {
		switch o.kind {
		case 'g':
			prefix = append(prefix, o.group)
		case 'a':
			for _, a := range o.attrs {
				fields[applyPrefix(prefix, a.Key)] = a.Value.Any()
			}
		}
	}
	r.Attrs(func(a slog.Attr) bool {
		// Record-time attrs are scoped into whatever groups are
		// currently active at the END of the chain.
		fields[applyPrefix(prefix, a.Key)] = a.Value.Any()
		return true
	})

	return Entry{
		Time:    r.Time,
		Level:   r.Level.String(),
		Message: r.Message,
		Fields:  fields,
	}
}

func applyPrefix(prefix []string, key string) string {
	for i := len(prefix) - 1; i >= 0; i-- {
		key = prefix[i] + "." + key
	}
	return key
}
