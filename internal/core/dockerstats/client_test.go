package dockerstats

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Disabled mode — empty socket path yields a nil client; method calls
// return ErrDisabled. Callers rely on this for graceful degradation.
func TestNewClient_Disabled(t *testing.T) {
	c, err := NewClient("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != nil {
		t.Fatalf("expected nil client when socketPath empty, got %v", c)
	}

	// Calling methods on a nil receiver must return ErrDisabled, not panic.
	if _, err := c.ContainerStats(context.Background(), "anything"); err != ErrDisabled {
		t.Errorf("expected ErrDisabled on nil client, got %v", err)
	}
	if _, err := c.ContainerStatsByService(context.Background(), "anything"); err != ErrDisabled {
		t.Errorf("expected ErrDisabled on nil client, got %v", err)
	}
}

func TestContainerNameFor(t *testing.T) {
	c := &Client{overrides: map[string]string{"Haul": "haul-prod"}}
	cases := []struct{ in, want string }{
		{"Haul", "haul-prod"},   // override wins
		{"Pilot", "pilot"},      // lowercase by default
		{"PRISM", "prism"},      // lowercase by default
	}
	for _, tc := range cases {
		if got := c.containerNameFor(tc.in); got != tc.want {
			t.Errorf("containerNameFor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// CPU rate cache — locks down the delta formula. If anyone reverts to the
// inline precpu_stats approach, this test fails immediately because cold
// start returns 0 and the second call would over-report against absolute
// zero baselines.
func TestCPURateCache_FirstCallReturnsZero(t *testing.T) {
	c := newCPURateCache(30 * time.Second)
	got := c.observe("a", time.Now(), 100, 1000, 4, 4)
	if got != 0 {
		t.Errorf("cold start cpu = %v, want 0", got)
	}
}

func TestCPURateCache_ComputesPercentFromDelta(t *testing.T) {
	c := newCPURateCache(30 * time.Second)
	t0 := time.Now()
	c.observe("a", t0, 100, 1000, 4, 4)
	got := c.observe("a", t0.Add(2*time.Second), 200, 2000, 4, 4)
	// (100/1000) * 4 * 100 = 40
	if got < 39.9 || got > 40.1 {
		t.Errorf("cpu rate = %v, want ~40", got)
	}
}

func TestCPURateCache_StaleEntryReturnsZero(t *testing.T) {
	c := newCPURateCache(1 * time.Second)
	t0 := time.Now()
	c.observe("a", t0, 100, 1000, 4, 4)
	got := c.observe("a", t0.Add(5*time.Second), 200, 2000, 4, 4)
	if got != 0 {
		t.Errorf("stale cpu rate = %v, want 0 (TTL not honored)", got)
	}
}

func TestCPURateCache_CounterResetReturnsZero(t *testing.T) {
	// Container restart — counters reset to small numbers; we should not
	// emit a negative or huge percentage.
	c := newCPURateCache(30 * time.Second)
	t0 := time.Now()
	c.observe("a", t0, 5_000_000, 50_000_000, 4, 4)
	got := c.observe("a", t0.Add(1*time.Second), 100, 1000, 4, 4)
	if got != 0 {
		t.Errorf("after counter reset = %v, want 0", got)
	}
}

func TestCPURateCache_FallsBackToPercpuLen(t *testing.T) {
	// Some Docker versions don't populate online_cpus; fall back to the
	// length of percpu_usage so we still return a reasonable percentage.
	c := newCPURateCache(30 * time.Second)
	t0 := time.Now()
	c.observe("a", t0, 100, 1000, 0, 8) // online=0, percpu_len=8
	got := c.observe("a", t0.Add(2*time.Second), 200, 2000, 0, 8)
	// (100/1000) * 8 * 100 = 80
	if got < 79.9 || got > 80.1 {
		t.Errorf("fallback cpus = %v, want ~80", got)
	}
}

func TestMemUsage_SubtractsCache(t *testing.T) {
	s := &statsResponse{}
	s.MemoryStats.Usage = 1_000_000
	s.MemoryStats.Stats = map[string]uint64{"cache": 200_000}
	if got := memUsage(s); got != 800_000 {
		t.Errorf("memUsage = %d, want 800000", got)
	}
}

func TestSumNetIO(t *testing.T) {
	s := &statsResponse{Networks: map[string]netData{
		"eth0": {RxBytes: 100, TxBytes: 50},
		"eth1": {RxBytes: 200, TxBytes: 75},
	}}
	rx, tx := sumNetIO(s)
	if rx != 300 || tx != 125 {
		t.Errorf("sumNetIO = (%d, %d), want (300, 125)", rx, tx)
	}
}

func TestBlockIO(t *testing.T) {
	s := &statsResponse{}
	s.BlkioStats.IoServiceBytesRecursive = []struct {
		Op    string `json:"op"`
		Value uint64 `json:"value"`
	}{
		{Op: "Read", Value: 100},
		{Op: "Write", Value: 200},
		{Op: "read", Value: 50}, // case-insensitive
	}
	if got := blockIO(s, "read"); got != 150 {
		t.Errorf("blockIO read = %d, want 150", got)
	}
	if got := blockIO(s, "write"); got != 200 {
		t.Errorf("blockIO write = %d, want 200", got)
	}
}

// Rate cache: first observation returns 0,0 (no baseline). Second call
// returns the delta divided by elapsed time.
func TestNetRateCache_FirstCallReturnsZero(t *testing.T) {
	c := newNetRateCache(30 * time.Second)
	t0 := time.Now()
	rx, tx := c.observe("a", t0, 1000, 500)
	if rx != 0 || tx != 0 {
		t.Errorf("first observe = (%v, %v), want (0, 0)", rx, tx)
	}
}

func TestNetRateCache_ComputesRate(t *testing.T) {
	c := newNetRateCache(30 * time.Second)
	t0 := time.Now()
	c.observe("a", t0, 1000, 500)
	rx, tx := c.observe("a", t0.Add(2*time.Second), 3000, 1500)
	// (3000-1000)/2 = 1000, (1500-500)/2 = 500
	if rx != 1000 || tx != 500 {
		t.Errorf("rate = (%v, %v), want (1000, 500)", rx, tx)
	}
}

func TestNetRateCache_StaleEntryReturnsZero(t *testing.T) {
	c := newNetRateCache(1 * time.Second)
	t0 := time.Now()
	c.observe("a", t0, 1000, 500)
	rx, tx := c.observe("a", t0.Add(5*time.Second), 3000, 1500)
	if rx != 0 || tx != 0 {
		t.Errorf("stale rate = (%v, %v), want (0, 0) — TTL not honored", rx, tx)
	}
}

func TestNetRateCache_CounterResetIsNotNegative(t *testing.T) {
	// A container restart resets cumulative counters. Don't emit negative
	// rates; just return 0 for that direction.
	c := newNetRateCache(30 * time.Second)
	t0 := time.Now()
	c.observe("a", t0, 5000, 5000)
	rx, tx := c.observe("a", t0.Add(1*time.Second), 100, 50)
	if rx != 0 || tx != 0 {
		t.Errorf("after counter reset = (%v, %v), want (0, 0)", rx, tx)
	}
}

func TestNetRateCache_EvictsStaleEntries(t *testing.T) {
	c := newNetRateCache(1 * time.Second)
	t0 := time.Now()
	c.observe("old", t0, 100, 100)

	// After TTL has passed, observing a different key should drop "old".
	c.observe("new", t0.Add(5*time.Second), 1, 1)

	c.mu.Lock()
	_, oldStillThere := c.entries["old"]
	c.mu.Unlock()
	if oldStillThere {
		t.Error("expected stale 'old' entry to be evicted")
	}
}

// End-to-end: spin up a fake docker daemon over HTTP, point the client at
// it, assert the parsed Stats matches what came back. We swap the unix
// transport for a TCP one because httptest can't speak unix sockets cleanly.
func TestContainerStats_EndToEnd(t *testing.T) {
	statsResp := map[string]any{
		"name": "/haul",
		"read": "2024-01-01T00:00:00Z",
		"cpu_stats": map[string]any{
			"cpu_usage":         map[string]any{"total_usage": 200, "percpu_usage": []int{1, 2, 3, 4}},
			"system_cpu_usage":  2000,
			"online_cpus":       4,
		},
		"precpu_stats": map[string]any{
			"cpu_usage":         map[string]any{"total_usage": 100},
			"system_cpu_usage":  1000,
		},
		"memory_stats": map[string]any{
			"usage": 1_000_000,
			"limit": 4_000_000,
			"stats": map[string]uint64{"cache": 200_000},
		},
		"networks": map[string]any{
			"eth0": map[string]any{"rx_bytes": 1000, "tx_bytes": 500},
		},
		"blkio_stats": map[string]any{
			"io_service_bytes_recursive": []map[string]any{
				{"op": "Read", "value": 4096},
				{"op": "Write", "value": 8192},
			},
		},
		"pids_stats": map[string]any{"current": 12},
	}
	inspectResp := map[string]any{
		"State": map[string]any{
			"Health": map[string]any{"Status": "healthy"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/stats"):
			_ = json.NewEncoder(w).Encode(statsResp)
		case strings.HasSuffix(r.URL.Path, "/json"):
			_ = json.NewEncoder(w).Encode(inspectResp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := &Client{
		http:      srv.Client(),
		baseURL:   "http://" + u.Host,
		rateCache: newNetRateCache(30 * time.Second),
		cpuCache:  newCPURateCache(30 * time.Second),
	}

	// First call: cold start — CPU% is 0 but everything else populates.
	first, err := c.ContainerStats(context.Background(), "haul")
	if err != nil {
		t.Fatalf("ContainerStats first: %v", err)
	}
	if first.CPUPercent != 0 {
		t.Errorf("first CPUPercent = %v, want 0 (cold start)", first.CPUPercent)
	}
	// Second call: now we have a baseline, CPU% is real. Same canned
	// response so total/system are unchanged → delta is zero → 0%.
	stats, err := c.ContainerStats(context.Background(), "haul")
	if err != nil {
		t.Fatalf("ContainerStats second: %v", err)
	}
	if stats.Name != "haul" {
		t.Errorf("Name = %q, want haul (note: leading slash trimmed)", stats.Name)
	}
	if stats.CPUPercent != 0 {
		t.Errorf("CPUPercent on identical samples = %v, want 0", stats.CPUPercent)
	}
	if stats.MemUsageBytes != 800_000 {
		t.Errorf("MemUsageBytes = %d, want 800000 (cache subtracted)", stats.MemUsageBytes)
	}
	if stats.MemLimitBytes != 4_000_000 {
		t.Errorf("MemLimitBytes = %d", stats.MemLimitBytes)
	}
	if stats.NetRxBytes != 1000 || stats.NetTxBytes != 500 {
		t.Errorf("Net = (%d, %d)", stats.NetRxBytes, stats.NetTxBytes)
	}
	if stats.BlockReadBytes != 4096 || stats.BlockWriteBytes != 8192 {
		t.Errorf("Block = (%d, %d)", stats.BlockReadBytes, stats.BlockWriteBytes)
	}
	if stats.PIDs != 12 {
		t.Errorf("PIDs = %d", stats.PIDs)
	}
	if stats.HealthStatus != "healthy" {
		t.Errorf("HealthStatus = %q", stats.HealthStatus)
	}
}

func TestContainerStats_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := &Client{
		http:      srv.Client(),
		baseURL:   "http://" + u.Host,
		rateCache: newNetRateCache(30 * time.Second),
		cpuCache:  newCPURateCache(30 * time.Second),
	}

	_, err := c.ContainerStats(context.Background(), "ghost")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got %v", err)
	}
}
