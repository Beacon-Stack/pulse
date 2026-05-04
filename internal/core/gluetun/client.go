// Package gluetun provides a thin HTTP client for the Gluetun container's
// control server (https://github.com/qdm12/gluetun-wiki/blob/main/setup/advanced/control-server.md).
//
// Pulse's dashboard surfaces VPN state — connected/IP/country/port-forwarded
// — alongside the rest of the stack. Disabled mode (nil baseURL) yields a
// nil *Client; callers must handle nil gracefully so the dashboard hides
// the VPN panel when no Gluetun is configured.
package gluetun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	baseURL string
	logger  *slog.Logger
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
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("gluetun GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("gluetun GET %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Response shapes — only fields we actually use.

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
