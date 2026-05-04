// Package dockerstats provides a thin client over the Docker Engine API for
// reading per-container resource statistics. Used by Pulse's dashboard to
// surface CPU / memory / network / block-IO for each registered service.
//
// Implemented directly against the Engine API over a Unix socket rather than
// pulling in the full docker/docker SDK, which would add tens of MB of
// transitive dependencies. The three endpoints we need are stable and
// well-documented.
//
// Disabled mode: if the socket path is empty, NewClient returns (nil, nil)
// — every consumer must check for nil before calling. This is the
// graceful-degradation path: if the operator hasn't opted in by mounting
// /var/run/docker.sock and setting PULSE_DOCKER_SOCKET, Pulse still works,
// container fields just render as null in the dashboard.
//
// Security: even a read-only docker socket grants effective root on the host.
// The env-gate is intentional — operators must opt in.
package dockerstats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrDisabled is returned when a method is called on a nil/disabled client.
var ErrDisabled = errors.New("dockerstats: disabled (no socket configured)")

// Stats is the subset of Docker container stats Pulse cares about.
// Cumulative counters are returned alongside derived per-second rates.
type Stats struct {
	Name            string  `json:"name"`
	CPUPercent      float64 `json:"cpu_percent"`
	MemUsageBytes   uint64  `json:"mem_usage_bytes"`
	MemLimitBytes   uint64  `json:"mem_limit_bytes"`
	NetRxBytes      uint64  `json:"net_rx_bytes"`
	NetTxBytes      uint64  `json:"net_tx_bytes"`
	NetRxRateBps    float64 `json:"net_rx_rate_bps"`
	NetTxRateBps    float64 `json:"net_tx_rate_bps"`
	BlockReadBytes  uint64  `json:"block_read_bytes"`
	BlockWriteBytes uint64  `json:"block_write_bytes"`
	PIDs            uint64  `json:"pids"`
	HealthStatus    string  `json:"health_status"` // "healthy", "starting", "unhealthy", "" (no healthcheck)
}

// Client talks to the Docker Engine API over a Unix socket.
// The zero value is invalid; use NewClient.
type Client struct {
	http       *http.Client
	baseURL    string
	overrides  map[string]string // logical service name → container name
	rateCache  *netRateCache
	cpuCache   *cpuRateCache
}

// NewClient returns a Docker Engine API client backed by the given Unix socket.
// If socketPath is empty, returns (nil, nil) — callers should treat nil as
// "disabled" and skip container stats. Any other error indicates a real
// problem (socket unreachable, permission denied, etc.).
func NewClient(socketPath string, nameOverrides map[string]string) (*Client, error) {
	if socketPath == "" {
		return nil, nil
	}
	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	return &Client{
		http:      &http.Client{Transport: transport, Timeout: 5 * time.Second},
		baseURL:   "http://docker",
		overrides: nameOverrides,
		rateCache: newNetRateCache(30 * time.Second),
		cpuCache:  newCPURateCache(30 * time.Second),
	}, nil
}

// ContainerStatsByService maps a logical service name (as registered with
// Pulse) to a container name and fetches its stats. The mapping rule is
// lowercase service name, with PULSE_CONTAINER_NAMES overrides taking
// precedence (e.g. "Haul" → "haul-prod" when overridden).
func (c *Client) ContainerStatsByService(ctx context.Context, serviceName string) (*Stats, error) {
	if c == nil {
		return nil, ErrDisabled
	}
	return c.ContainerStats(ctx, c.containerNameFor(serviceName))
}

// containerNameFor resolves a service name to its container name.
func (c *Client) containerNameFor(serviceName string) string {
	if override, ok := c.overrides[serviceName]; ok {
		return override
	}
	return strings.ToLower(serviceName)
}

// ContainerStats fetches a snapshot of stats for the given container name.
// The Docker API's `stats?stream=false` endpoint returns cumulative counters
// plus a precpu snapshot — one HTTP call gives us everything for CPU%.
// Network rates are derived from the local cache against a previous snapshot.
func (c *Client) ContainerStats(ctx context.Context, containerName string) (*Stats, error) {
	if c == nil {
		return nil, ErrDisabled
	}

	// Stats endpoint.
	statsRaw, err := c.fetchContainerStats(ctx, containerName)
	if err != nil {
		return nil, err
	}

	// Inspect endpoint — for health status. Optional; if it fails we
	// just return empty health and continue.
	health := c.fetchContainerHealth(ctx, containerName)

	now := time.Now()
	rxTotal, txTotal := sumNetIO(statsRaw)
	rxRate, txRate := c.rateCache.observe(containerName, now, rxTotal, txTotal)

	// Compute CPU% from successive snapshots cached locally rather than
	// from Docker's inline precpu_stats — when we ask for stream=false &
	// one-shot=true (which we do, to avoid the 1s blocking call), Docker
	// zero-fills precpu_stats and the inline calculation always yields 0.
	cpu := c.cpuCache.observe(
		containerName, now,
		statsRaw.CPUStats.CPUUsage.TotalUsage,
		statsRaw.CPUStats.SystemUsage,
		statsRaw.CPUStats.OnlineCPUs,
		uint32(len(statsRaw.CPUStats.CPUUsage.PercpuUsage)),
	)

	return &Stats{
		Name:            strings.TrimPrefix(statsRaw.Name, "/"),
		CPUPercent:      cpu,
		MemUsageBytes:   memUsage(statsRaw),
		MemLimitBytes:   statsRaw.MemoryStats.Limit,
		NetRxBytes:      rxTotal,
		NetTxBytes:      txTotal,
		NetRxRateBps:    rxRate,
		NetTxRateBps:    txRate,
		BlockReadBytes:  blockIO(statsRaw, "read"),
		BlockWriteBytes: blockIO(statsRaw, "write"),
		PIDs:            statsRaw.PidsStats.Current,
		HealthStatus:    health,
	}, nil
}

func (c *Client) fetchContainerStats(ctx context.Context, name string) (*statsResponse, error) {
	u := c.baseURL + "/containers/" + url.PathEscape(name) + "/stats?stream=false&one-shot=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker stats request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("container %q not found", name)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("docker stats: HTTP %d", resp.StatusCode)
	}
	var sr statsResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode stats: %w", err)
	}
	return &sr, nil
}

// ContainerEnv returns the effective environment of a container as a map.
// Used by the dashboard to pull metadata Gluetun's control server doesn't
// expose (e.g. VPN_SERVICE_PROVIDER). Returns ErrDisabled when the client
// is nil; an empty (non-nil) map when the container exists but has no env;
// an error on lookup failure.
func (c *Client) ContainerEnv(ctx context.Context, containerName string) (map[string]string, error) {
	if c == nil {
		return nil, ErrDisabled
	}
	u := c.baseURL + "/containers/" + url.PathEscape(containerName) + "/json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker inspect %s: %w", containerName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("container %q not found", containerName)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("docker inspect %s: HTTP %d", containerName, resp.StatusCode)
	}
	var ir struct {
		Config struct {
			Env []string `json:"Env"`
		} `json:"Config"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ir); err != nil {
		return nil, fmt.Errorf("decode inspect: %w", err)
	}
	out := make(map[string]string, len(ir.Config.Env))
	for _, kv := range ir.Config.Env {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			continue
		}
		out[kv[:i]] = kv[i+1:]
	}
	return out, nil
}

func (c *Client) fetchContainerHealth(ctx context.Context, name string) string {
	u := c.baseURL + "/containers/" + url.PathEscape(name) + "/json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return ""
	}
	var ir struct {
		State struct {
			Health *struct {
				Status string `json:"Status"`
			} `json:"Health"`
		} `json:"State"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ir); err != nil {
		return ""
	}
	if ir.State.Health == nil {
		return ""
	}
	return ir.State.Health.Status
}

// statsResponse mirrors the docker /stats response shape (fields we use only).
type statsResponse struct {
	Name        string  `json:"name"`
	Read        string  `json:"read"`
	CPUStats    cpuData `json:"cpu_stats"`
	PreCPUStats cpuData `json:"precpu_stats"`
	MemoryStats memData `json:"memory_stats"`
	Networks    map[string]netData `json:"networks"`
	BlkioStats  blkioData `json:"blkio_stats"`
	PidsStats   struct {
		Current uint64 `json:"current"`
	} `json:"pids_stats"`
}

type cpuData struct {
	CPUUsage struct {
		TotalUsage  uint64   `json:"total_usage"`
		PercpuUsage []uint64 `json:"percpu_usage"`
	} `json:"cpu_usage"`
	SystemUsage uint64 `json:"system_cpu_usage"`
	OnlineCPUs  uint32 `json:"online_cpus"`
}

type memData struct {
	Usage uint64            `json:"usage"`
	Limit uint64            `json:"limit"`
	Stats map[string]uint64 `json:"stats"`
}

type netData struct {
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
}

type blkioData struct {
	IoServiceBytesRecursive []struct {
		Op    string `json:"op"`
		Value uint64 `json:"value"`
	} `json:"io_service_bytes_recursive"`
}

// cpuRateCache holds the previous CPU snapshot per container and computes
// the delta against each new sample. Docker's inline precpu_stats can't be
// used here because we request `?stream=false&one-shot=true` to avoid the
// 1s blocking call — that returns zero precpu, making any inline
// calculation degenerate to 0. The cache makes successive 2s polls
// produce real CPU% numbers.
type cpuRateCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]cpuSample
}

type cpuSample struct {
	at         time.Time
	totalUsage uint64
	sysUsage   uint64
	cpus       uint32
}

func newCPURateCache(ttl time.Duration) *cpuRateCache {
	return &cpuRateCache{ttl: ttl, entries: make(map[string]cpuSample)}
}

// observe records a new CPU snapshot and returns the % vs the previous
// snapshot. Returns 0 on cold start or when the cached entry has expired
// (e.g. the container was restarted and counters reset).
func (c *cpuRateCache) observe(key string, now time.Time, total, sys uint64, onlineCPUs, percpuLen uint32) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	prev, ok := c.entries[key]
	c.entries[key] = cpuSample{at: now, totalUsage: total, sysUsage: sys, cpus: onlineCPUs}

	for k, e := range c.entries {
		if now.Sub(e.at) > c.ttl {
			delete(c.entries, k)
		}
	}

	if !ok || now.Sub(prev.at) > c.ttl {
		return 0
	}
	if total < prev.totalUsage || sys < prev.sysUsage {
		// Counter reset (container restart); next sample will be valid.
		return 0
	}
	cpuDelta := float64(total - prev.totalUsage)
	sysDelta := float64(sys - prev.sysUsage)
	if sysDelta <= 0 {
		return 0
	}
	cpus := float64(onlineCPUs)
	if cpus == 0 {
		cpus = float64(percpuLen)
	}
	if cpus == 0 {
		cpus = 1
	}
	return (cpuDelta / sysDelta) * cpus * 100.0
}

// memUsage subtracts cache from raw usage when stats provide it
// (matches docker CLI behavior — the kernel page cache isn't really "used").
func memUsage(s *statsResponse) uint64 {
	if cache, ok := s.MemoryStats.Stats["cache"]; ok && s.MemoryStats.Usage >= cache {
		return s.MemoryStats.Usage - cache
	}
	return s.MemoryStats.Usage
}

func sumNetIO(s *statsResponse) (rx, tx uint64) {
	for _, n := range s.Networks {
		rx += n.RxBytes
		tx += n.TxBytes
	}
	return rx, tx
}

func blockIO(s *statsResponse, op string) uint64 {
	var total uint64
	for _, e := range s.BlkioStats.IoServiceBytesRecursive {
		if strings.EqualFold(e.Op, op) {
			total += e.Value
		}
	}
	return total
}

// netRateCache derives per-second network rates from successive cumulative
// snapshots. Per-container; entries older than ttl are evicted on read.
type netRateCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]netRateEntry
}

type netRateEntry struct {
	at time.Time
	rx uint64
	tx uint64
}

func newNetRateCache(ttl time.Duration) *netRateCache {
	return &netRateCache{ttl: ttl, entries: make(map[string]netRateEntry)}
}

// observe records a new (rx, tx) snapshot for the given key and returns the
// per-second rate against the previous snapshot. Returns 0,0 on the first
// call or when the previous snapshot has expired.
func (c *netRateCache) observe(key string, now time.Time, rx, tx uint64) (rxRate, txRate float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	prev, ok := c.entries[key]
	c.entries[key] = netRateEntry{at: now, rx: rx, tx: tx}

	// Evict stale neighbors so the map doesn't grow unboundedly when
	// containers come and go.
	for k, e := range c.entries {
		if now.Sub(e.at) > c.ttl {
			delete(c.entries, k)
		}
	}

	if !ok || now.Sub(prev.at) > c.ttl {
		return 0, 0
	}
	dt := now.Sub(prev.at).Seconds()
	if dt <= 0 {
		return 0, 0
	}
	if rx >= prev.rx {
		rxRate = float64(rx-prev.rx) / dt
	}
	if tx >= prev.tx {
		txRate = float64(tx-prev.tx) / dt
	}
	return rxRate, txRate
}
