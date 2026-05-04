// Package loki ships Beacon logs to a Grafana Loki instance via
// /loki/api/v1/push. Loki is the most common self-hosted target in
// homelab deployments — easy to compose, low ceremony, plays
// nicely with Grafana for ad-hoc querying.
//
// Design notes:
//
//   - Async, batched, lossy on shutdown. Logs are NOT critical-path:
//     dropping a few records on a network blip is preferable to
//     blocking app code on a remote write. The batcher accumulates
//     up to MaxBatchSize records or MaxBatchInterval seconds (whichever
//     first) and flushes in one POST.
//   - One stream per service+level. Loki indexes "labels" (the keys
//     you can filter on cheaply); attribute values go in the
//     unindexed log line as JSON. We intentionally keep the label
//     set tiny — `service` and `level` — to avoid the high-
//     cardinality footgun where every (hash, episode) combination
//     creates its own stream.
//   - Auth: optional X-Scope-OrgID header for multi-tenant setups,
//     plus optional Basic auth user/pass for managed Loki services.
//
// Operator config (via env vars in the host service's main.go):
//
//	BEACON_LOG_LOKI_URL       — push endpoint base URL (e.g. http://loki:3100)
//	BEACON_LOG_LOKI_TENANT    — optional X-Scope-OrgID
//	BEACON_LOG_LOKI_USER      — optional basic auth user
//	BEACON_LOG_LOKI_PASS      — optional basic auth password
package loki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Config controls the Loki plugin. Service is required so log lines
// are tagged correctly in Loki's label index.
type Config struct {
	Service string
	URL     string

	// Auth (all optional)
	TenantID     string
	BasicUser    string
	BasicPass    string

	// Batcher tuning. Defaults are reasonable for steady-state
	// homelab traffic; bump MaxBatchSize for very chatty services.
	MaxBatchSize     int           // default 200
	MaxBatchInterval time.Duration // default 2s
	Timeout          time.Duration // default 5s per push

	// Client overrides for testing.
	HTTPClient *http.Client
}

// Plugin implements log.Plugin. Constructed by New, registered via
// system.Add(plugin).
type Plugin struct {
	cfg     Config
	handler *handler
}

// New validates config and returns a started plugin. Returns an
// error when URL is empty (so the caller can decide whether to
// crash startup or just log a warning and continue without Loki).
func New(cfg Config) (*Plugin, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("loki: URL required")
	}
	if cfg.Service == "" {
		return nil, fmt.Errorf("loki: Service required")
	}
	if cfg.MaxBatchSize <= 0 {
		cfg.MaxBatchSize = 200
	}
	if cfg.MaxBatchInterval <= 0 {
		cfg.MaxBatchInterval = 2 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: cfg.Timeout}
	}

	h := &handler{
		cfg:    cfg,
		ch:     make(chan record, cfg.MaxBatchSize*2),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	go h.run()

	return &Plugin{cfg: cfg, handler: h}, nil
}

// Name returns "loki".
func (p *Plugin) Name() string { return "loki" }

// Handler returns the slog.Handler that captures records into the
// async batcher.
func (p *Plugin) Handler() slog.Handler { return p.handler }

// Close drains pending records up to a timeout. Returns the context
// error if the deadline is exceeded — the host service can decide
// whether to surface that to the operator.
func (p *Plugin) Close(ctx context.Context) error {
	close(p.handler.stopCh)
	select {
	case <-p.handler.doneCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ── slog.Handler implementation ────────────────────────────────────────────

type record struct {
	t     time.Time
	level slog.Level
	msg   string
	attrs map[string]any
}

type handler struct {
	cfg    Config
	ch     chan record
	stopCh chan struct{}
	doneCh chan struct{}

	mu     sync.RWMutex
	groups []string
	bound  []slog.Attr
}

func (h *handler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	rec := record{
		t:     r.Time,
		level: r.Level,
		msg:   r.Message,
		attrs: make(map[string]any, len(h.bound)+r.NumAttrs()),
	}
	for _, a := range h.bound {
		rec.attrs[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		key := a.Key
		for i := len(h.groups) - 1; i >= 0; i-- {
			key = h.groups[i] + "." + key
		}
		rec.attrs[key] = a.Value.Any()
		return true
	})

	// Non-blocking enqueue: dropping is acceptable when Loki is
	// down and the buffer's full. The alternative — blocking —
	// would freeze the application's hot path on a remote outage.
	select {
	case h.ch <- rec:
	default:
	}
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return &handler{
		cfg:    h.cfg,
		ch:     h.ch,
		stopCh: h.stopCh,
		doneCh: h.doneCh,
		groups: h.groups,
		bound:  append(append([]slog.Attr{}, h.bound...), attrs...),
	}
}

func (h *handler) WithGroup(name string) slog.Handler {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return &handler{
		cfg:    h.cfg,
		ch:     h.ch,
		stopCh: h.stopCh,
		doneCh: h.doneCh,
		groups: append(append([]string{}, h.groups...), name),
		bound:  h.bound,
	}
}

// ── Batcher loop ───────────────────────────────────────────────────────────

func (h *handler) run() {
	defer close(h.doneCh)

	batch := make([]record, 0, h.cfg.MaxBatchSize)
	tick := time.NewTicker(h.cfg.MaxBatchInterval)
	defer tick.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := h.push(batch); err != nil {
			// We use Default to log Loki failures so they show up
			// in stdout even when Loki is the broken sink. The
			// TeeHandler keeps stdout independent of Loki, so this
			// is safe — no infinite recursion.
			slog.Default().Warn("loki push failed", "error", err, "dropped", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-h.stopCh:
			// Drain channel best-effort, then flush + return.
			for {
				select {
				case r := <-h.ch:
					batch = append(batch, r)
				default:
					flush()
					return
				}
			}
		case r := <-h.ch:
			batch = append(batch, r)
			if len(batch) >= h.cfg.MaxBatchSize {
				flush()
			}
		case <-tick.C:
			flush()
		}
	}
}

// push sends the batch to Loki. Loki's push API expects:
//
//	{"streams":[{"stream":{<labels>},"values":[["<ts_ns>","<line>"], …]}]}
//
// We bucket by (service, level) so the label cardinality stays low.
func (h *handler) push(batch []record) error {
	type stream struct {
		Stream map[string]string `json:"stream"`
		Values [][2]string       `json:"values"`
	}
	type payload struct {
		Streams []stream `json:"streams"`
	}

	bucket := make(map[string]*stream)
	for _, r := range batch {
		levelStr := r.level.String()
		key := r.level.String()
		s, ok := bucket[key]
		if !ok {
			s = &stream{Stream: map[string]string{
				"service": h.cfg.Service,
				"level":   levelStr,
			}}
			bucket[key] = s
		}
		// Values: ["<ns>", "<line>"]. Line is JSON for richer
		// queries from Grafana (e.g. `| json | hash="abc"`).
		line := buildLine(r)
		s.Values = append(s.Values, [2]string{
			strconv.FormatInt(r.t.UnixNano(), 10),
			line,
		})
	}

	p := payload{}
	for _, s := range bucket {
		p.Streams = append(p.Streams, *s)
	}
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, h.cfg.URL+"/loki/api/v1/push", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if h.cfg.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", h.cfg.TenantID)
	}
	if h.cfg.BasicUser != "" {
		req.SetBasicAuth(h.cfg.BasicUser, h.cfg.BasicPass)
	}

	resp, err := h.cfg.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("loki returned %d", resp.StatusCode)
	}
	return nil
}

// buildLine flattens the record into a JSON object Loki indexes as
// the log line. Includes msg + attrs; the timestamp is in the
// outer Values slot so we don't duplicate it inside the line.
func buildLine(r record) string {
	out := make(map[string]any, len(r.attrs)+1)
	out["msg"] = r.msg
	for k, v := range r.attrs {
		out[k] = v
	}
	b, err := json.Marshal(out)
	if err != nil {
		return r.msg
	}
	return string(b)
}
