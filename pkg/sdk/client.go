// Package sdk provides a Go client for ecosystem services to register with
// Pulse, send heartbeats, discover peers, and pull managed resources
// (indexers, download clients, quality profiles, shared settings).
//
// Usage:
//
//	client, err := sdk.New(sdk.Config{
//	    PulseURL: "http://pulse:9696",
//	    APIKey:         "your-api-key",
//	    ServiceName:    "luminarr",
//	    ServiceType:    "media-manager",
//	    APIURL:         "http://luminarr:8282",
//	    HealthURL:      "http://luminarr:8282/health",
//	    Version:        "0.1.0",
//	    Capabilities:   []string{"supports_torrent", "supports_usenet"},
//	})
//	defer client.Close()
//
// The client auto-registers on creation and sends heartbeats at a configurable
// interval. Call Discover/Config methods to query the control plane.
package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Config holds the settings for connecting to Pulse.
type Config struct {
	// PulseURL is the base URL of the Pulse instance (e.g. "http://pulse:9696").
	PulseURL string

	// APIKey is the Pulse API key for authentication.
	APIKey string

	// ServiceName is the name this service registers under (e.g. "luminarr").
	ServiceName string

	// ServiceType categorizes the service (e.g. "media-manager", "download-client").
	ServiceType string

	// APIURL is the URL where this service's API is reachable.
	APIURL string

	// HealthURL is the URL for health checks. Optional.
	HealthURL string

	// Version is the service version string. Optional.
	Version string

	// Capabilities declares what this service supports. Optional.
	Capabilities []string

	// HeartbeatInterval controls how often heartbeats are sent. Default: 30s.
	HeartbeatInterval time.Duration

	// Logger is an optional structured logger. Falls back to slog.Default().
	Logger *slog.Logger

	// HTTPClient is an optional custom HTTP client. Falls back to http.DefaultClient.
	HTTPClient *http.Client
}

// Client is the Pulse SDK client. Create one with New().
type Client struct {
	cfg       Config
	serviceID string
	client    *http.Client
	logger    *slog.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new SDK client, registers the service with Pulse,
// and starts the heartbeat loop. Call Close() to stop the heartbeat and
// deregister (optional).
func New(cfg Config) (*Client, error) {
	if cfg.PulseURL == "" {
		return nil, fmt.Errorf("sdk: PulseURL is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("sdk: APIKey is required")
	}
	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("sdk: ServiceName is required")
	}
	if cfg.ServiceType == "" {
		return nil, fmt.Errorf("sdk: ServiceType is required")
	}
	if cfg.APIURL == "" {
		return nil, fmt.Errorf("sdk: APIURL is required")
	}

	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 30 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}

	c := &Client{
		cfg:    cfg,
		client: cfg.HTTPClient,
		logger: cfg.Logger,
	}

	// Register
	svc, err := c.register(context.Background())
	if err != nil {
		return nil, fmt.Errorf("sdk: registration failed: %w", err)
	}
	c.serviceID = svc.ID
	c.logger.Info("sdk: registered with pulse",
		"service_id", svc.ID,
		"name", svc.Name,
		"type", svc.Type,
	)

	// Start heartbeat
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.wg.Add(1)
	go c.heartbeatLoop(ctx)

	return c, nil
}

// ServiceID returns the ID assigned by Pulse during registration.
func (c *Client) ServiceID() string {
	return c.serviceID
}

// Close stops the heartbeat loop. It does NOT deregister — the service
// remains registered so Pulse can track it went offline via health checks.
func (c *Client) Close() {
	c.cancel()
	c.wg.Wait()
}

// Deregister stops the heartbeat and removes this service from Pulse.
func (c *Client) Deregister(ctx context.Context) error {
	c.Close()
	return c.doRequest(ctx, "DELETE", fmt.Sprintf("/api/v1/services/%s", c.serviceID), nil, nil)
}

// ── Discovery ────────────────────────────────────────────────────────────────

// Service represents a registered service returned by discovery queries.
type Service struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	APIURL       string   `json:"api_url"`
	HealthURL    string   `json:"health_url"`
	Version      string   `json:"version"`
	Status       string   `json:"status"`
	LastSeen     string   `json:"last_seen"`
	Registered   string   `json:"registered"`
	Capabilities []string `json:"capabilities"`
}

// DiscoverByType returns all services of a given type (e.g. "download-client").
func (c *Client) DiscoverByType(ctx context.Context, serviceType string) ([]Service, error) {
	var out []Service
	err := c.doRequest(ctx, "GET", "/api/v1/services/discover?type="+url.QueryEscape(serviceType), nil, &out)
	return out, err
}

// DiscoverByCapability returns all services declaring a given capability.
func (c *Client) DiscoverByCapability(ctx context.Context, capability string) ([]Service, error) {
	var out []Service
	err := c.doRequest(ctx, "GET", "/api/v1/services/discover?capability="+url.QueryEscape(capability), nil, &out)
	return out, err
}

// DiscoverAll returns all registered services.
func (c *Client) DiscoverAll(ctx context.Context) ([]Service, error) {
	var out []Service
	err := c.doRequest(ctx, "GET", "/api/v1/services", nil, &out)
	return out, err
}

// ── Indexers ─────────────────────────────────────────────────────────────────

// Indexer represents an indexer assigned to this service.
type Indexer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Enabled   bool   `json:"enabled"`
	Priority  int    `json:"priority"`
	URL       string `json:"url"`
	Settings  string `json:"settings"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// MyIndexers returns the indexers assigned to this service by Pulse.
func (c *Client) MyIndexers(ctx context.Context) ([]Indexer, error) {
	var out []Indexer
	err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/services/%s/indexers", c.serviceID), nil, &out)
	return out, err
}

// ── Download Clients ─────────────────────────────────────────────────────────

// DownloadClient represents a centrally managed download client.
type DownloadClient struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`      // qbittorrent, deluge, transmission, sabnzbd, nzbget
	Protocol  string `json:"protocol"`  // torrent, usenet
	Enabled   bool   `json:"enabled"`
	Priority  int    `json:"priority"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	UseSSL    bool   `json:"use_ssl"`
	Username  string `json:"username"`
	Category  string `json:"category"`
	Directory string `json:"directory"`
	Settings  string `json:"settings"`
}

// MyDownloadClients returns download clients available to this service.
func (c *Client) MyDownloadClients(ctx context.Context) ([]DownloadClient, error) {
	var out []DownloadClient
	err := c.doRequest(ctx, "GET", "/api/v1/download-clients", nil, &out)
	return out, err
}

// ── Quality Profiles ─────────────────────────────────────────────────────────

// QualityProfile represents a centrally managed quality profile pushed from Pulse.
// The JSON blob fields (CutoffJSON, QualitiesJSON, UpgradeUntilJSON) are opaque
// to Pulse — consumers parse them into their own domain types.
type QualityProfile struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	CutoffJSON           string  `json:"cutoff_json"`
	QualitiesJSON        string  `json:"qualities_json"`
	UpgradeAllowed       bool    `json:"upgrade_allowed"`
	UpgradeUntilJSON     *string `json:"upgrade_until_json,omitempty"`
	MinCustomFormatScore int     `json:"min_custom_format_score"`
	UpgradeUntilCFScore  int     `json:"upgrade_until_cf_score"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

// MyQualityProfiles returns quality profiles available to this service.
// Services call this on their sync loop to receive centrally managed profiles.
func (c *Client) MyQualityProfiles(ctx context.Context) ([]QualityProfile, error) {
	var out []QualityProfile
	err := c.doRequest(ctx, "GET", "/api/v1/quality-profiles", nil, &out)
	return out, err
}

// ── Shared Media Handling Settings ───────────────────────────────────────────

// SharedSettings is the filesystem/handling settings that apply uniformly
// across all media-manager services. Services overlay these onto their local
// media_management row on every sync tick.
type SharedSettings struct {
	ColonReplacement    string `json:"colon_replacement"`
	ImportExtraFiles    bool   `json:"import_extra_files"`
	ExtraFileExtensions string `json:"extra_file_extensions"`
	RenameFiles         bool   `json:"rename_files"`
	UpdatedAt           string `json:"updated_at"`
}

// MySharedSettings returns the shared media handling settings from Pulse.
func (c *Client) MySharedSettings(ctx context.Context) (*SharedSettings, error) {
	var out SharedSettings
	if err := c.doRequest(ctx, "GET", "/api/v1/shared-settings", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── Internal ─────────────────────────────────────────────────────────────────

func (c *Client) register(ctx context.Context) (*Service, error) {
	body := map[string]any{
		"name":         c.cfg.ServiceName,
		"type":         c.cfg.ServiceType,
		"api_url":      c.cfg.APIURL,
		"health_url":   c.cfg.HealthURL,
		"version":      c.cfg.Version,
		"capabilities": c.cfg.Capabilities,
	}
	var svc Service
	if err := c.doRequest(ctx, "POST", "/api/v1/services/register", body, &svc); err != nil {
		return nil, err
	}
	return &svc, nil
}

func (c *Client) heartbeatLoop(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(c.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(ctx); err != nil {
				c.logger.Warn("sdk: heartbeat failed", "error", err)
			}
		}
	}
}

func (c *Client) sendHeartbeat(ctx context.Context) error {
	return c.doRequest(ctx, "PUT", fmt.Sprintf("/api/v1/services/%s/heartbeat", c.serviceID), nil, nil)
}

func (c *Client) doRequest(ctx context.Context, method, path string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.cfg.PulseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("X-Api-Key", c.cfg.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 204 {
		return nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}

	return nil
}
