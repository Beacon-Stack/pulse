// Package gluetun provides a thin HTTP client for the Gluetun container's
// control server (https://github.com/qdm12/gluetun-wiki/blob/main/setup/advanced/control-server.md).
//
// Pulse's dashboard surfaces VPN state — connected/IP/country/port-forwarded
// — alongside the rest of the stack. Disabled mode (nil baseURL) yields a
// nil *Client; callers must handle nil gracefully so the dashboard hides
// the VPN panel when no Gluetun is configured.
package gluetun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrDisabled is returned when a method is called on a nil/disabled client.
var ErrDisabled = errors.New("gluetun: disabled (no URL configured)")

// Status is the aggregated VPN status Pulse exposes on the dashboard.
// Zero values render as "unknown" in the UI — every field is best-effort.
type Status struct {
	// Reachable is true if at least one control-server endpoint responded.
	// Distinguishes "configured but unreachable" from real disconnection.
	Reachable         bool   `json:"reachable"`
	Connected         bool   `json:"connected"`
	PublicIP          string `json:"public_ip"`
	Country           string `json:"country"`
	Region            string `json:"region"`
	City              string `json:"city"`
	Hostname          string `json:"hostname"`
	Organization      string `json:"organization"`
	PortForwarded     int    `json:"port_forwarded"`
	DNSStatus         string `json:"dns_status"`
	Provider          string `json:"provider"`
	KillSwitchEnabled bool   `json:"kill_switch_enabled"`
	Version           string `json:"version"`
}

// Client talks to a Gluetun control server.
type Client struct {
	http    *http.Client
	slow    *http.Client // rotate ops: control server lags during reconnect
	baseURL string
	logger  *slog.Logger
	// apiKey, when set, is sent as X-API-Key on every request. Routes
	// that can read credentials or change settings (/v1/vpn/settings)
	// belong behind a Gluetun role with apikey auth.
	apiKey string
}

// SetAPIKey sets the key sent as X-API-Key on every control-server
// request. Empty sends none.
func (c *Client) SetAPIKey(key string) {
	if c != nil {
		c.apiKey = key
	}
}

// do sends req with the API key header, if one is set.
func (c *Client) do(hc *http.Client, req *http.Request) (*http.Response, error) {
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	return hc.Do(req)
}

// NewClient returns a Gluetun client. If baseURL is empty, returns nil —
// callers should treat that as "disabled" and skip VPN status reporting.
// baseURL must include scheme (e.g. "http://vpn:8000").
// A nil logger is fine; calls fall back to slog.Default().
func NewClient(baseURL string, logger *slog.Logger) *Client {
	if baseURL == "" {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		http:    &http.Client{Timeout: 1500 * time.Millisecond},
		slow:    &http.Client{Timeout: 15 * time.Second},
		baseURL: strings.TrimRight(baseURL, "/"),
		logger:  logger,
	}
}

// Status aggregates several Gluetun endpoints in parallel, returning a
// best-effort snapshot. A slow or unreachable endpoint never blocks the
// others past 1s — the corresponding fields are left as zero values
// rather than failing the whole call.
//
// The Reachable field on the returned Status is true if at least one
// endpoint responded successfully — callers can use it to distinguish
// "VPN not configured" from "we configured it but Gluetun's control server
// isn't responding".
func (c *Client) Status(ctx context.Context) (*Status, error) {
	if c == nil {
		return nil, ErrDisabled
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		status   Status
		successes int
		failures  int
	)

	// /v1/publicip/ip — defines "connected": if we have a public IP, the
	// VPN tunnel is up. Also gives geo metadata for the UI.
	wg.Add(1)
	go func() {
		defer wg.Done()
		var body publicIPResponse
		if err := c.getJSON(ctx, "/v1/publicip/ip", &body); err != nil {
			c.recordFailure(&mu, &failures, "/v1/publicip/ip", err)
			return
		}
		c.recordSuccess(&mu, &successes)
		mu.Lock()
		defer mu.Unlock()
		status.PublicIP = body.IP
		status.Country = body.Country
		status.Region = body.Region
		status.City = body.City
		status.Hostname = body.Hostname
		status.Organization = body.Organization
		status.Connected = body.IP != ""
	}()

	// /v1/openvpn/portforwarded — only meaningful for providers that
	// support port forwarding (PIA, ProtonVPN). Others return 0.
	wg.Add(1)
	go func() {
		defer wg.Done()
		var body portForwardedResponse
		if err := c.getJSON(ctx, "/v1/openvpn/portforwarded", &body); err != nil {
			c.recordFailure(&mu, &failures, "/v1/openvpn/portforwarded", err)
			return
		}
		c.recordSuccess(&mu, &successes)
		mu.Lock()
		status.PortForwarded = body.Port
		mu.Unlock()
	}()

	// /v1/dns/status — "on" / "off" / "error".
	wg.Add(1)
	go func() {
		defer wg.Done()
		var body dnsStatusResponse
		if err := c.getJSON(ctx, "/v1/dns/status", &body); err != nil {
			c.recordFailure(&mu, &failures, "/v1/dns/status", err)
			return
		}
		c.recordSuccess(&mu, &successes)
		mu.Lock()
		status.DNSStatus = body.Status
		mu.Unlock()
	}()

	// /v1/openvpn/settings — provider name and a few flags. The
	// kill-switch setting is the firewall block-outbound default.
	wg.Add(1)
	go func() {
		defer wg.Done()
		var body openvpnSettingsResponse
		if err := c.getJSON(ctx, "/v1/openvpn/settings", &body); err != nil {
			c.recordFailure(&mu, &failures, "/v1/openvpn/settings", err)
			return
		}
		c.recordSuccess(&mu, &successes)
		mu.Lock()
		if body.Provider != "" {
			status.Provider = body.Provider
		}
		// Gluetun's firewall is on by default; the only way to disable
		// the kill-switch is to flip the firewall off entirely.
		status.KillSwitchEnabled = body.Firewall == nil || *body.Firewall
		mu.Unlock()
	}()

	// /v1/version — Gluetun build version.
	wg.Add(1)
	go func() {
		defer wg.Done()
		var body versionResponse
		if err := c.getJSON(ctx, "/v1/version", &body); err != nil {
			c.recordFailure(&mu, &failures, "/v1/version", err)
			return
		}
		c.recordSuccess(&mu, &successes)
		mu.Lock()
		status.Version = body.Version
		mu.Unlock()
	}()

	wg.Wait()

	// Reachable if any endpoint succeeded. Lets the frontend distinguish
	// "VPN not configured" from "configured but unreachable" — without it,
	// the dashboard would render an empty card with "unknown provider"
	// and "disconnected" forever, making it look like the VPN is down
	// when really Pulse just can't talk to Gluetun's control server.
	status.Reachable = successes > 0

	if successes == 0 && failures > 0 {
		c.logger.Warn("gluetun: all control-server calls failed; check reachability and HTTP_CONTROL_SERVER_ADDRESS",
			"base_url", c.baseURL, "failures", failures)
	}

	return &status, nil
}

func (c *Client) recordSuccess(mu *sync.Mutex, n *int) {
	mu.Lock()
	*n++
	mu.Unlock()
}

func (c *Client) recordFailure(mu *sync.Mutex, n *int, path string, err error) {
	mu.Lock()
	*n++
	mu.Unlock()
	c.logger.Debug("gluetun: endpoint failed", "path", path, "error", err)
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	// Each call gets its own 1s deadline so one slow endpoint can't drag
	// out the whole aggregation.
	cctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(cctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.do(c.http, req)
	if err != nil {
		return fmt.Errorf("gluetun GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("gluetun GET %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// SetOpenVPNStatus starts or stops the OpenVPN tunnel via the control
// server. status must be "running" or "stopped". Uses the slow client —
// stopping/starting the tunnel can take several seconds to acknowledge.
func (c *Client) SetOpenVPNStatus(ctx context.Context, status string) error {
	if c == nil {
		return ErrDisabled
	}
	body, _ := json.Marshal(map[string]string{"status": status})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/v1/openvpn/status", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(c.slow, req)
	if err != nil {
		return fmt.Errorf("gluetun PUT /v1/openvpn/status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("gluetun PUT /v1/openvpn/status: HTTP %d", resp.StatusCode)
	}
	return nil
}

// PublicIP returns the tunnel's current public IP (empty while the
// tunnel is down or the IP-fetch hasn't completed). Uses the slow client
// — after a reconnect the control server's IP endpoint can lag well past
// the 1s Status budget.
func (c *Client) PublicIP(ctx context.Context) (string, error) {
	if c == nil {
		return "", ErrDisabled
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/publicip/ip", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.do(c.slow, req)
	if err != nil {
		return "", fmt.Errorf("gluetun GET /v1/publicip/ip: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("gluetun GET /v1/publicip/ip: HTTP %d", resp.StatusCode)
	}
	var body publicIPResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.IP, nil
}

// VPNSelection is the part of Gluetun's VPN settings Pulse reads: which
// provider, which regions it may pick servers from, and whether port
// forwarding is on. The full settings document also carries the VPN
// credentials; they are never decoded.
type VPNSelection struct {
	Provider       string
	Regions        []string
	PortForwarding bool
}

// VPNSettings reads the provider, region filter and port-forwarding flag
// from GET /v1/vpn/settings.
func (c *Client) VPNSettings(ctx context.Context) (VPNSelection, error) {
	if c == nil {
		return VPNSelection{}, ErrDisabled
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/vpn/settings", nil)
	if err != nil {
		return VPNSelection{}, err
	}
	resp, err := c.do(c.slow, req)
	if err != nil {
		return VPNSelection{}, fmt.Errorf("gluetun GET /v1/vpn/settings: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return VPNSelection{}, fmt.Errorf("gluetun GET /v1/vpn/settings: HTTP %d", resp.StatusCode)
	}
	var body vpnSettingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return VPNSelection{}, fmt.Errorf("gluetun GET /v1/vpn/settings: %w", err)
	}
	sel := VPNSelection{
		Provider: body.Provider.Name,
		Regions:  body.Provider.ServerSelection.Regions,
	}
	if pf := body.Provider.PortForwarding.Enabled; pf != nil {
		sel.PortForwarding = *pf
	}
	return sel, nil
}

// SetRegions limits server selection to regions and applies it live:
// Gluetun validates the change and reconnects the tunnel. It lasts until
// the container restarts, when its environment applies again — see
// Rotator.KeepPreferredRegion.
func (c *Client) SetRegions(ctx context.Context, regions []string) error {
	if c == nil {
		return ErrDisabled
	}
	body, _ := json.Marshal(map[string]any{
		"provider": map[string]any{
			"server_selection": map[string]any{"regions": regions},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/v1/vpn/settings", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(c.slow, req)
	if err != nil {
		return fmt.Errorf("gluetun PUT /v1/vpn/settings: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		// Gluetun's validation message ("region X is not valid ...") is
		// the useful part of a 400.
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("gluetun PUT /v1/vpn/settings: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// Response shapes — only fields we actually use.

type vpnSettingsResponse struct {
	Provider struct {
		Name            string `json:"name"`
		ServerSelection struct {
			Regions []string `json:"regions"`
		} `json:"server_selection"`
		PortForwarding struct {
			Enabled *bool `json:"enabled"`
		} `json:"port_forwarding"`
	} `json:"provider"`
}

type publicIPResponse struct {
	IP           string `json:"public_ip"`
	Country      string `json:"country"`
	Region       string `json:"region"`
	City         string `json:"city"`
	Hostname     string `json:"hostname"`
	Organization string `json:"organization"`
}

type portForwardedResponse struct {
	Port int `json:"port"`
}

type dnsStatusResponse struct {
	Status string `json:"status"`
}

type openvpnSettingsResponse struct {
	Provider string `json:"provider"`
	// Firewall ptr distinguishes "absent in response" from "explicitly false".
	// Gluetun returns this as a top-level bool when known.
	Firewall *bool `json:"firewall"`
}

type versionResponse struct {
	Version string `json:"version"`
}
