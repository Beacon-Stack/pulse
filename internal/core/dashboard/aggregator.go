// Package dashboard aggregates per-service health, container resource usage,
// and VPN status into a single payload for Pulse's UI dashboard.
//
// Stitches three data sources:
//   - registry.Service — the canonical list of registered services
//   - dockerstats.Client — per-container CPU / memory / net (optional)
//   - gluetun.Client — VPN status (optional)
// plus per-service /api/v1/stats and /api/v1/system/runtime fetched live.
//
// Every dependency may be nil (Docker socket not mounted, no Gluetun
// configured); the aggregator degrades gracefully and returns whatever
// data it has.
package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/beacon-stack/pulse/internal/core/dockerstats"
	"github.com/beacon-stack/pulse/internal/core/gluetun"
	"github.com/beacon-stack/pulse/internal/core/registry"
)

// fanoutTimeout caps any single per-service fetch. A slow service must not
// drag out the whole overview poll.
const fanoutTimeout = 2 * time.Second

// Aggregator builds dashboard payloads.
type Aggregator struct {
	Registry        *registry.Service
	Docker          *dockerstats.Client
	Gluetun         *gluetun.Client
	HTTP            *http.Client
	GluetunService  string // logical name used for the VPN container's stats card; default "vpn"
}

// New returns an aggregator with sane defaults.
func New(reg *registry.Service, docker *dockerstats.Client, glu *gluetun.Client) *Aggregator {
	return &Aggregator{
		Registry:       reg,
		Docker:         docker,
		Gluetun:        glu,
		HTTP:           &http.Client{Timeout: fanoutTimeout},
		GluetunService: "vpn",
	}
}

// activeListLimit caps how many entries the overview returns inline. The
// total count is reported separately so the UI can render a "+ N more"
// expansion when truncated.
const activeListLimit = 5

// Overview is the small payload polled every 2s by the dashboard.
type Overview struct {
	Services             []ServiceSummary `json:"services"`
	Haul                 *HaulSummary     `json:"haul,omitempty"`
	VPN                  *VPNSummary      `json:"vpn,omitempty"`
	ActiveDownloads      []ActiveDownload `json:"active_downloads,omitempty"`
	ActiveDownloadsTotal int              `json:"active_downloads_total,omitempty"`
	ActiveImports        []ActiveImport   `json:"active_imports,omitempty"`
	ActiveImportsTotal   int              `json:"active_imports_total,omitempty"`
}

// ActiveDownload is a torrent currently downloading on Haul (or any future
// download client). The dashboard renders the top N by activity with
// progress, rate, peers, ETA.
type ActiveDownload struct {
	ServiceID    string  `json:"service_id"`
	ServiceName  string  `json:"service_name"`
	Name         string  `json:"name"`
	Progress     float64 `json:"progress"` // 0-1
	DownloadRate int64   `json:"download_rate"`
	UploadRate   int64   `json:"upload_rate"`
	Peers        int     `json:"peers"`
	ETASeconds   int64   `json:"eta_seconds"`
	Status       string  `json:"status"`
}

// ActiveImport is an item in flight on a media manager (Pilot/Prism queue).
// Bundles items from both managers into a single feed so the dashboard
// can show "what's being processed" without per-service silos.
type ActiveImport struct {
	ServiceID    string  `json:"service_id"`
	ServiceName  string  `json:"service_name"`
	Title        string  `json:"title"`
	Status       string  `json:"status"`
	Size         int64   `json:"size"`
	Downloaded   int64   `json:"downloaded_bytes"`
	Progress     float64 `json:"progress"` // 0-1, derived
	GrabbedAt    string  `json:"grabbed_at"`
}

// ServiceSummary is a per-service card in the dashboard grid.
type ServiceSummary struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Type      string           `json:"type"`
	Status    string           `json:"status"`
	Version   string           `json:"version"`
	Container *ContainerStats  `json:"container,omitempty"`
}

// ContainerStats is the dashboard-relevant subset of dockerstats.Stats.
type ContainerStats struct {
	CPUPercent      float64 `json:"cpu_percent"`
	MemUsageBytes   uint64  `json:"mem_usage_bytes"`
	MemLimitBytes   uint64  `json:"mem_limit_bytes"`
	NetRxBytes      uint64  `json:"net_rx_bytes"`
	NetTxBytes      uint64  `json:"net_tx_bytes"`
	NetRxRateBps    float64 `json:"net_rx_rate_bps"`
	NetTxRateBps    float64 `json:"net_tx_rate_bps"`
	BlockReadBytes  uint64  `json:"block_read_bytes"`
	BlockWriteBytes uint64  `json:"block_write_bytes"`
	HealthStatus    string  `json:"health_status"`
}

// HaulSummary is the throughput card on the overview.
type HaulSummary struct {
	DownloadSpeed   int64 `json:"download_speed"`
	UploadSpeed     int64 `json:"upload_speed"`
	ActiveDownloads int   `json:"active_downloads"`
	ActiveUploads   int   `json:"active_uploads"`
	PeersConnected  int   `json:"peers_connected"`
}

// VPNSummary is the small VPN card on the overview.
type VPNSummary struct {
	// Reachable: Pulse got at least one successful response from Gluetun's
	// control server. False means we couldn't talk to it at all — the
	// frontend hides the panel rather than showing a misleading
	// "disconnected" state.
	Reachable     bool   `json:"reachable"`
	Connected     bool   `json:"connected"`
	PublicIP      string `json:"public_ip"`
	Country       string `json:"country"`
	PortForwarded int    `json:"port_forwarded"`
	Provider      string `json:"provider"`
	DNSStatus     string `json:"dns_status"`
}

// RuntimeStats is the per-service Go-runtime stats from /api/v1/system/runtime.
type RuntimeStats struct {
	Goroutines    int    `json:"goroutines"`
	HeapAlloc     uint64 `json:"heap_alloc_bytes"`
	HeapInUse     uint64 `json:"heap_in_use_bytes"`
	HeapObjects   uint64 `json:"heap_objects"`
	NumGC         uint32 `json:"num_gc"`
	LastGCPauseNs uint64 `json:"last_gc_pause_ns"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	GoVersion     string `json:"go_version"`
	GoOS          string `json:"goos"`
	GoArch        string `json:"goarch"`
	NumCPU        int    `json:"num_cpu"`
	Hostname      string `json:"hostname"`
}

// EnvEntry is one environment variable row from /api/v1/system/env.
type EnvEntry struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Redacted bool   `json:"redacted"`
}

// LogEntry is one log line from /api/v1/system/logs.
type LogEntry struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

// ServiceDetail is the full payload backing the drill-down drawer.
type ServiceDetail struct {
	Service   ServiceSummary  `json:"service"`
	Container *ContainerStats `json:"container,omitempty"`
	Runtime   *RuntimeStats   `json:"runtime,omitempty"`
	Env       []EnvEntry      `json:"env,omitempty"`
	Logs      []LogEntry      `json:"logs,omitempty"`
	Specifics map[string]any  `json:"specifics,omitempty"`
}

// Overview builds the dashboard's per-2s payload. Best-effort across all
// data sources — if Docker is disabled or Gluetun is unreachable, the
// affected fields are simply nil.
func (a *Aggregator) Overview(ctx context.Context) (*Overview, error) {
	if a == nil || a.Registry == nil {
		return nil, errors.New("aggregator: registry not configured")
	}

	services, err := a.Registry.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing services: %w", err)
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		out      = &Overview{Services: make([]ServiceSummary, len(services))}
	)

	for i, svc := range services {
		i, svc := i, svc
		wg.Add(1)
		go func() {
			defer wg.Done()
			summary := ServiceSummary{
				ID:      svc.ID,
				Name:    svc.Name,
				Type:    svc.Type,
				Status:  svc.Status,
				Version: svc.Version,
			}
			summary.Container = a.containerStatsFor(ctx, svc.Name)

			// Per-type fan-out for active items. Pulse's overview merges
			// these into single cross-service feeds; the alternative
			// (per-service panels) doesn't scale as we add services.
			isDownloadClient := strings.EqualFold(svc.Type, "download-client") || strings.EqualFold(svc.Name, "Haul")
			isMediaManager := strings.EqualFold(svc.Type, "media-manager") ||
				strings.EqualFold(svc.Name, "Pilot") || strings.EqualFold(svc.Name, "Prism")

			if isDownloadClient {
				if h := a.fetchHaulStats(ctx, svc.ApiUrl, svc.ApiKey); h != nil {
					mu.Lock()
					out.Haul = h
					mu.Unlock()
				}
				if downloads := a.fetchActiveDownloads(ctx, svc.ID, svc.Name, svc.ApiUrl, svc.ApiKey); len(downloads) > 0 {
					mu.Lock()
					out.ActiveDownloads = append(out.ActiveDownloads, downloads...)
					mu.Unlock()
				}
			}

			if isMediaManager {
				if imports := a.fetchActiveImports(ctx, svc.ID, svc.Name, svc.ApiUrl, svc.ApiKey); len(imports) > 0 {
					mu.Lock()
					out.ActiveImports = append(out.ActiveImports, imports...)
					mu.Unlock()
				}
			}

			mu.Lock()
			out.Services[i] = summary
			mu.Unlock()
		}()
	}

	// Gluetun status fetched in parallel with the service fan-out.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if a.Gluetun == nil {
			return
		}
		gctx, cancel := context.WithTimeout(ctx, fanoutTimeout)
		defer cancel()
		st, err := a.Gluetun.Status(gctx)
		if err != nil || st == nil {
			return
		}
		// Don't surface anything if we couldn't talk to Gluetun. The
		// frontend treats nil VPN as "VPN not available" rather than
		// rendering an empty disconnected card.
		if !st.Reachable {
			return
		}
		// Gluetun's /v1/openvpn/settings doesn't include the provider name
		// — VPN_SERVICE_PROVIDER lives only in the gluetun container's
		// env. If Docker stats are enabled, inspect the container and
		// pull it from there so the dashboard doesn't show "unknown
		// provider" when we already have the answer.
		provider := st.Provider
		if provider == "" && a.Docker != nil {
			ictx, icancel := context.WithTimeout(ctx, fanoutTimeout)
			env, err := a.Docker.ContainerEnv(ictx, a.GluetunService)
			icancel()
			if err == nil {
				if p, ok := env["VPN_SERVICE_PROVIDER"]; ok {
					provider = p
				}
			}
		}
		mu.Lock()
		out.VPN = &VPNSummary{
			Reachable:     st.Reachable,
			Connected:     st.Connected,
			PublicIP:      st.PublicIP,
			Country:       st.Country,
			PortForwarded: st.PortForwarded,
			Provider:      provider,
			DNSStatus:     st.DNSStatus,
		}
		mu.Unlock()
	}()

	wg.Wait()

	// Truncate to top N. Sort each list by activity (downloads by rate,
	// imports by status priority then grabbed-at) so the truncation
	// keeps the most useful entries.
	sort.SliceStable(out.ActiveDownloads, func(i, j int) bool {
		return out.ActiveDownloads[i].DownloadRate > out.ActiveDownloads[j].DownloadRate
	})
	out.ActiveDownloadsTotal = len(out.ActiveDownloads)
	if len(out.ActiveDownloads) > activeListLimit {
		out.ActiveDownloads = out.ActiveDownloads[:activeListLimit]
	}

	sort.SliceStable(out.ActiveImports, func(i, j int) bool {
		// "downloading" before "queued"; within a status, newest first.
		ai, aj := importSortKey(out.ActiveImports[i].Status), importSortKey(out.ActiveImports[j].Status)
		if ai != aj {
			return ai < aj
		}
		return out.ActiveImports[i].GrabbedAt > out.ActiveImports[j].GrabbedAt
	})
	out.ActiveImportsTotal = len(out.ActiveImports)
	if len(out.ActiveImports) > activeListLimit {
		out.ActiveImports = out.ActiveImports[:activeListLimit]
	}

	return out, nil
}

// importSortKey orders queue items so the most-actionable show first.
// Lower number sorts first.
func importSortKey(status string) int {
	switch strings.ToLower(status) {
	case "downloading":
		return 0
	case "queued":
		return 1
	case "paused":
		return 2
	case "failed":
		return 3
	case "completed":
		return 4
	default:
		return 5
	}
}

// ServiceDetail backs the drill-down drawer for a single service.
func (a *Aggregator) ServiceDetail(ctx context.Context, serviceID string) (*ServiceDetail, error) {
	if a == nil || a.Registry == nil {
		return nil, errors.New("aggregator: registry not configured")
	}

	svc, err := a.Registry.Get(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	out := &ServiceDetail{Service: ServiceSummary{
		ID:      svc.ID,
		Name:    svc.Name,
		Type:    svc.Type,
		Status:  svc.Status,
		Version: svc.Version,
	}}

	var wg sync.WaitGroup
	var mu sync.Mutex

	wg.Add(1)
	go func() {
		defer wg.Done()
		out.Container = a.containerStatsFor(ctx, svc.Name)
		// Mirror onto the summary so card and drawer agree.
		mu.Lock()
		out.Service.Container = out.Container
		mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		out.Runtime = a.fetchRuntime(ctx, svc.ApiUrl, svc.ApiKey)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		env := a.fetchEnv(ctx, svc.ApiUrl, svc.ApiKey)
		mu.Lock()
		out.Env = env
		mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		logs := a.fetchLogs(ctx, svc.ApiUrl, svc.ApiKey, 100)
		mu.Lock()
		out.Logs = logs
		mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		spec := a.fetchSpecifics(ctx, svc.Name, svc.Type, svc.ApiUrl, svc.ApiKey)
		mu.Lock()
		out.Specifics = spec
		mu.Unlock()
	}()

	wg.Wait()
	return out, nil
}

// containerStatsFor returns the Docker stats for the named service, mapping
// service name → container name via the dockerstats client. Returns nil if
// Docker is disabled or the lookup fails.
func (a *Aggregator) containerStatsFor(ctx context.Context, serviceName string) *ContainerStats {
	if a.Docker == nil {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, fanoutTimeout)
	defer cancel()
	s, err := a.Docker.ContainerStatsByService(cctx, serviceName)
	if err != nil || s == nil {
		return nil
	}
	return &ContainerStats{
		CPUPercent:      s.CPUPercent,
		MemUsageBytes:   s.MemUsageBytes,
		MemLimitBytes:   s.MemLimitBytes,
		NetRxBytes:      s.NetRxBytes,
		NetTxBytes:      s.NetTxBytes,
		NetRxRateBps:    s.NetRxRateBps,
		NetTxRateBps:    s.NetTxRateBps,
		BlockReadBytes:  s.BlockReadBytes,
		BlockWriteBytes: s.BlockWriteBytes,
		HealthStatus:    s.HealthStatus,
	}
}

// fetchHaulStats pulls /api/v1/stats off the named Haul service URL.
func (a *Aggregator) fetchHaulStats(ctx context.Context, baseURL, apiKey string) *HaulSummary {
	if baseURL == "" {
		return nil
	}
	var body struct {
		DownloadSpeed   int64 `json:"download_speed"`
		UploadSpeed     int64 `json:"upload_speed"`
		ActiveDownloads int   `json:"active_downloads"`
		ActiveUploads   int   `json:"active_uploads"`
		PeersConnected  int   `json:"peers_connected"`
	}
	if err := a.serviceGetJSON(ctx, baseURL, "/api/v1/stats", apiKey, &body); err != nil {
		return nil
	}
	return &HaulSummary{
		DownloadSpeed:   body.DownloadSpeed,
		UploadSpeed:     body.UploadSpeed,
		ActiveDownloads: body.ActiveDownloads,
		ActiveUploads:   body.ActiveUploads,
		PeersConnected:  body.PeersConnected,
	}
}

// fetchActiveDownloads pulls /api/v1/torrents and filters to in-progress
// items (progress < 1, not in a terminal state). Returns one
// ActiveDownload per torrent; the caller merges and truncates across
// services.
func (a *Aggregator) fetchActiveDownloads(ctx context.Context, serviceID, serviceName, baseURL, apiKey string) []ActiveDownload {
	if baseURL == "" {
		return nil
	}
	var raw []struct {
		Name         string  `json:"name"`
		Status       string  `json:"status"`
		Progress     float64 `json:"progress"`
		DownloadRate int64   `json:"download_rate"`
		UploadRate   int64   `json:"upload_rate"`
		Peers        int     `json:"peers"`
		ETA          int64   `json:"eta"`
	}
	if err := a.serviceGetJSON(ctx, baseURL, "/api/v1/torrents", apiKey, &raw); err != nil {
		return nil
	}
	out := make([]ActiveDownload, 0, len(raw))
	for _, t := range raw {
		if !torrentIsActive(t.Status, t.Progress) {
			continue
		}
		out = append(out, ActiveDownload{
			ServiceID:    serviceID,
			ServiceName:  serviceName,
			Name:         t.Name,
			Progress:     t.Progress,
			DownloadRate: t.DownloadRate,
			UploadRate:   t.UploadRate,
			Peers:        t.Peers,
			ETASeconds:   t.ETA,
			Status:       t.Status,
		})
	}
	return out
}

// torrentIsActive — anything not finished, not paused/errored, and not
// purely seeding is "active" for dashboard purposes.
func torrentIsActive(status string, progress float64) bool {
	if progress >= 1.0 {
		return false
	}
	switch strings.ToLower(status) {
	case "paused", "error", "errored", "stopped", "completed", "seeding":
		return false
	}
	return true
}

// fetchActiveImports pulls /api/v1/queue from a media manager and filters
// out terminal statuses. The grab-history shape on Pilot/Prism is shared
// (both use the queue.Item contract from the SDK), so one decoder works
// for both.
func (a *Aggregator) fetchActiveImports(ctx context.Context, serviceID, serviceName, baseURL, apiKey string) []ActiveImport {
	if baseURL == "" {
		return nil
	}
	// Pilot/Prism have a /api/v1/queue endpoint; some versions return the
	// list directly, others wrap it under {records: [...]}. Try both.
	var direct []struct {
		ID              string `json:"id"`
		ReleaseTitle    string `json:"release_title"`
		Status          string `json:"status"`
		Size            int64  `json:"size"`
		DownloadedBytes int64  `json:"downloaded_bytes"`
		GrabbedAt       string `json:"grabbed_at"`
	}
	if err := a.serviceGetJSON(ctx, baseURL, "/api/v1/queue", apiKey, &direct); err != nil {
		// Fall back to wrapped shape.
		var wrapped struct {
			Records []struct {
				ID              string `json:"id"`
				ReleaseTitle    string `json:"release_title"`
				Status          string `json:"status"`
				Size            int64  `json:"size"`
				DownloadedBytes int64  `json:"downloaded_bytes"`
				GrabbedAt       string `json:"grabbed_at"`
			} `json:"records"`
		}
		if err := a.serviceGetJSON(ctx, baseURL, "/api/v1/queue", apiKey, &wrapped); err != nil {
			return nil
		}
		direct = wrapped.Records
	}
	out := make([]ActiveImport, 0, len(direct))
	for _, q := range direct {
		if !queueItemIsActive(q.Status) {
			continue
		}
		var progress float64
		if q.Size > 0 {
			progress = float64(q.DownloadedBytes) / float64(q.Size)
			if progress > 1 {
				progress = 1
			}
		}
		out = append(out, ActiveImport{
			ServiceID:   serviceID,
			ServiceName: serviceName,
			Title:       q.ReleaseTitle,
			Status:      q.Status,
			Size:        q.Size,
			Downloaded:  q.DownloadedBytes,
			Progress:    progress,
			GrabbedAt:   q.GrabbedAt,
		})
	}
	return out
}

func queueItemIsActive(status string) bool {
	switch strings.ToLower(status) {
	case "completed", "imported", "failed", "cancelled":
		return false
	}
	return true
}

// fetchRuntime pulls /api/v1/system/runtime off any Beacon service.
func (a *Aggregator) fetchRuntime(ctx context.Context, baseURL, apiKey string) *RuntimeStats {
	if baseURL == "" {
		return nil
	}
	var body RuntimeStats
	if err := a.serviceGetJSON(ctx, baseURL, "/api/v1/system/runtime", apiKey, &body); err != nil {
		return nil
	}
	return &body
}

// fetchEnv pulls /api/v1/system/env off any Beacon service. Secrets are
// already redacted by the remote endpoint; we just pass them through.
func (a *Aggregator) fetchEnv(ctx context.Context, baseURL, apiKey string) []EnvEntry {
	if baseURL == "" {
		return nil
	}
	var body []EnvEntry
	if err := a.serviceGetJSON(ctx, baseURL, "/api/v1/system/env", apiKey, &body); err != nil {
		return nil
	}
	return body
}

// fetchLogs pulls the last `limit` log entries from a Beacon service's
// in-memory ring buffer. Returns nil on error so the drawer just hides the
// section rather than displaying a misleading empty list.
func (a *Aggregator) fetchLogs(ctx context.Context, baseURL, apiKey string, limit int) []LogEntry {
	if baseURL == "" {
		return nil
	}
	var body []LogEntry
	path := fmt.Sprintf("/api/v1/system/logs?limit=%d", limit)
	if err := a.serviceGetJSON(ctx, baseURL, path, apiKey, &body); err != nil {
		return nil
	}
	return body
}

// fetchSpecifics fetches a service-type-specific payload for the drawer.
// Currently:
//   - Haul → /api/v1/stats and /api/v1/torrents (top 5)
//   - Pilot / Prism → /api/v1/queue
//   - everything else → empty
func (a *Aggregator) fetchSpecifics(ctx context.Context, name, svcType, baseURL, apiKey string) map[string]any {
	if baseURL == "" {
		return nil
	}
	out := map[string]any{}
	switch {
	case strings.EqualFold(name, "Haul") || strings.EqualFold(svcType, "download-client"):
		var stats map[string]any
		if err := a.serviceGetJSON(ctx, baseURL, "/api/v1/stats", apiKey, &stats); err == nil {
			out["stats"] = stats
		}
		var torrents []map[string]any
		if err := a.serviceGetJSON(ctx, baseURL, "/api/v1/torrents?limit=5", apiKey, &torrents); err == nil {
			out["torrents"] = torrents
		}
	case strings.EqualFold(name, "Pilot"), strings.EqualFold(name, "Prism"),
		strings.EqualFold(svcType, "media-manager"):
		var queue map[string]any
		if err := a.serviceGetJSON(ctx, baseURL, "/api/v1/queue", apiKey, &queue); err == nil {
			out["queue"] = queue
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// serviceGetJSON makes a GET request to a registered service's API and
// decodes JSON into out. Always uses fanoutTimeout. Apikey is sent as
// X-Api-Key. Errors (non-2xx, decode failure, network) are returned as-is
// — callers typically discard the error and let the field be nil.
func (a *Aggregator) serviceGetJSON(ctx context.Context, baseURL, path, apiKey string, out any) error {
	cctx, cancel := context.WithTimeout(ctx, fanoutTimeout)
	defer cancel()

	url := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if apiKey != "" {
		req.Header.Set("X-Api-Key", apiKey)
	}
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
