package gluetun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/beacon-stack/pulse/internal/events"
)

// RegionCatalog reads the server lists Gluetun keeps on disk
// (/gluetun/servers/<provider>.json), mounted read-only into Pulse.
// Gluetun's control server doesn't list servers, and these files are
// exactly what it validates a region change against.
type RegionCatalog struct {
	Dir string // empty: no catalog, region changes unavailable
}

// Regions returns the provider's region names, sorted. With
// portForwardOnly, only regions with a server that supports port
// forwarding — a region without one would leave the torrent client
// unreachable.
func (rc RegionCatalog) Regions(provider string, portForwardOnly bool) ([]string, error) {
	if rc.Dir == "" {
		return nil, errors.New("no Gluetun servers directory configured")
	}
	if provider == "" || strings.ContainsAny(provider, `/\`) || strings.Contains(provider, "..") {
		return nil, fmt.Errorf("unusable provider name %q", provider)
	}
	raw, err := os.ReadFile(filepath.Join(rc.Dir, strings.ToLower(provider)+".json"))
	if err != nil {
		return nil, fmt.Errorf("reading server list: %w", err)
	}
	var doc struct {
		Servers []struct {
			Region      string `json:"region"`
			PortForward bool   `json:"port_forward"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decoding server list: %w", err)
	}
	seen := map[string]bool{}
	var out []string
	for _, srv := range doc.Servers {
		if srv.Region == "" || seen[srv.Region] || (portForwardOnly && !srv.PortForward) {
			continue
		}
		seen[srv.Region] = true
		out = append(out, srv.Region)
	}
	sort.Strings(out)
	return out, nil
}

// regionClient is the slice of Client region changes need.
type regionClient interface {
	rotateClient
	VPNSettings(ctx context.Context) (VPNSelection, error)
	SetRegions(ctx context.Context, regions []string) error
}

// RegionPreference persists the region the user picked, so it can be put
// back after the VPN container restarts with its environment's regions.
type RegionPreference interface {
	Get(ctx context.Context) (string, error) // "" when none chosen
	Set(ctx context.Context, region string) error
}

// RegionInfo is what the dashboard's region picker shows.
type RegionInfo struct {
	Available bool     `json:"available"`
	Reason    string   `json:"reason,omitempty"` // why not available
	Provider  string   `json:"provider,omitempty"`
	Current   []string `json:"current"`
	Preferred string   `json:"preferred,omitempty"`
	Regions   []string `json:"regions"`
	// PortForwardOnly: the list is limited to regions that can forward a
	// port, because port forwarding is on.
	PortForwardOnly bool `json:"port_forward_only"`
}

// RegionResult reports a region change.
type RegionResult struct {
	Region  string `json:"region"`
	OldIP   string `json:"old_ip"`
	NewIP   string `json:"new_ip"`
	Changed bool   `json:"changed"`
}

// ErrRegionsUnavailable means region changes aren't set up (no servers
// directory, or Gluetun refused the settings routes).
var ErrRegionsUnavailable = errors.New("region changes are not available")

// EnableRegions turns on region changes. Without it the region methods
// report ErrRegionsUnavailable.
func (r *Rotator) EnableRegions(client regionClient, catalog RegionCatalog, prefs RegionPreference) {
	r.regionClient = client
	r.catalog = catalog
	r.prefs = prefs
}

// Regions describes the current and available regions.
func (r *Rotator) Regions(ctx context.Context) RegionInfo {
	if r.regionClient == nil || r.catalog.Dir == "" {
		return RegionInfo{Reason: "Pulse isn't set up to change the VPN region (no Gluetun API key or servers directory)."}
	}
	sel, err := r.regionClient.VPNSettings(ctx)
	if err != nil {
		r.logger.Warn("vpn regions: reading Gluetun settings failed", "error", err)
		return RegionInfo{Reason: "Gluetun didn't share its VPN settings. Check that Pulse's API key has the /v1/vpn/settings routes."}
	}
	regions, err := r.catalog.Regions(sel.Provider, sel.PortForwarding)
	if err != nil {
		r.logger.Warn("vpn regions: reading server list failed", "provider", sel.Provider, "error", err)
		return RegionInfo{Provider: sel.Provider, Reason: "Pulse can't read Gluetun's server list."}
	}
	info := RegionInfo{
		Available:       true,
		Provider:        sel.Provider,
		Current:         canonicalRegions(sel.Regions, regions),
		Regions:         regions,
		PortForwardOnly: sel.PortForwarding,
	}
	if r.prefs != nil {
		info.Preferred, _ = r.prefs.Get(ctx)
	}
	return info
}

// ChangeRegion moves the tunnel to region, remembers the choice, and
// clears IP-bound scraper state. It shares rotation's lock and cool-down:
// both reconnect the tunnel, pause torrents and void Cloudflare sessions.
func (r *Rotator) ChangeRegion(ctx context.Context, region string) (RegionResult, error) {
	info := r.Regions(ctx)
	if !info.Available {
		return RegionResult{}, fmt.Errorf("%w: %s", ErrRegionsUnavailable, info.Reason)
	}
	canonical, ok := findRegion(info.Regions, region)
	if !ok {
		return RegionResult{}, fmt.Errorf("%q is not a region this VPN can use", region)
	}
	region = canonical
	if err := r.begin(); err != nil {
		return RegionResult{}, err
	}
	defer r.end()

	oldIP, err := r.client.PublicIP(ctx)
	if err != nil {
		return RegionResult{}, fmt.Errorf("reading current public IP: %w", err)
	}
	res := RegionResult{Region: region, OldIP: oldIP}
	r.logger.Info("vpn region: changing", "region", region, "old_ip", oldIP)
	if err := r.regionClient.SetRegions(ctx, []string{region}); err != nil {
		return res, fmt.Errorf("changing region: %w", err)
	}
	if r.prefs != nil {
		if err := r.prefs.Set(ctx, region); err != nil {
			// The change happened; only surviving a VPN restart is lost.
			r.logger.Warn("vpn region: could not save the choice", "error", err)
		}
	}
	newIP, err := r.waitForNewIP(ctx, oldIP)
	if err != nil {
		return res, err
	}
	res.NewIP = newIP
	res.Changed = newIP != oldIP
	r.afterReconnect(ctx, map[string]any{
		"old_ip": oldIP, "new_ip": newIP, "changed": res.Changed, "region": region,
	})
	r.logger.Info("vpn region: done", "region", region, "old_ip", oldIP, "new_ip", newIP)
	return res, nil
}

// KeepPreferredRegion puts the chosen region back whenever Gluetun is
// found using other regions — after the VPN container restarts, its
// environment's SERVER_REGIONS apply again. Checks every interval until
// ctx ends.
func (r *Rotator) KeepPreferredRegion(ctx context.Context, interval time.Duration) {
	if r.regionClient == nil || r.prefs == nil {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		r.reapplyPreferred(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (r *Rotator) reapplyPreferred(ctx context.Context) {
	want, err := r.prefs.Get(ctx)
	if err != nil || want == "" {
		return
	}
	sel, err := r.regionClient.VPNSettings(ctx)
	if err != nil {
		r.logger.Debug("vpn region: can't read Gluetun settings to check region", "error", err)
		return
	}
	// Gluetun reports regions lowercased ("ca toronto") while its server
	// list spells them "CA Toronto"; an exact compare would reconnect the
	// VPN on every check.
	if len(sel.Regions) == 1 && strings.EqualFold(sel.Regions[0], want) {
		return
	}
	if err := r.begin(); err != nil {
		return // a rotation or change is running; check again later
	}
	defer r.end()
	r.logger.Info("vpn region: putting back the chosen region (VPN restarted?)",
		"region", want, "gluetun_regions", sel.Regions)
	oldIP, _ := r.client.PublicIP(ctx)
	if err := r.regionClient.SetRegions(ctx, []string{want}); err != nil {
		r.logger.Warn("vpn region: putting back the chosen region failed", "region", want, "error", err)
		return
	}
	newIP, err := r.waitForNewIP(ctx, oldIP)
	if err != nil {
		r.logger.Warn("vpn region: tunnel didn't come back after re-applying region", "error", err)
		return
	}
	r.afterReconnect(ctx, map[string]any{
		"old_ip": oldIP, "new_ip": newIP, "changed": newIP != oldIP, "region": want, "reapplied": true,
	})
}

// findRegion finds name in the catalog's regions, ignoring case, and
// returns the catalog's spelling.
func findRegion(regions []string, name string) (string, bool) {
	for _, r := range regions {
		if strings.EqualFold(r, name) {
			return r, true
		}
	}
	return "", false
}

// canonicalRegions maps Gluetun's (lowercased) region names to the
// catalog's spelling, keeping any it can't match as given.
func canonicalRegions(current, catalog []string) []string {
	out := make([]string, 0, len(current))
	for _, c := range current {
		if r, ok := findRegion(catalog, c); ok {
			out = append(out, r)
		} else {
			out = append(out, c)
		}
	}
	return out
}

// waitForNewIP waits for the reconnected tunnel to report an IP other
// than oldIP. If it reports only oldIP until the budget runs out, that
// IP is returned (the provider can hand back the same server).
func (r *Rotator) waitForNewIP(ctx context.Context, oldIP string) (string, error) {
	deadline := time.Now().Add(r.pollBudget)
	last := ""
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(r.pollInterval):
		}
		ip, err := r.client.PublicIP(ctx)
		if err == nil && ip != "" {
			if ip != oldIP {
				return ip, nil
			}
			last = ip
		}
		if time.Now().After(deadline) {
			if last != "" {
				return last, nil
			}
			return "", fmt.Errorf("tunnel did not report a public IP within %s", r.pollBudget)
		}
	}
}

// afterReconnect clears IP-bound scraper state, re-warms Cloudflare
// sessions and announces the new IP. Shared by rotation and region change.
func (r *Rotator) afterReconnect(ctx context.Context, data map[string]any) {
	if r.clearScraperState != nil {
		r.clearScraperState()
	}
	if r.prewarm != nil {
		go r.prewarm(context.WithoutCancel(ctx))
	}
	if r.bus != nil {
		r.bus.Publish(ctx, events.Event{Type: events.TypeVPNIPRotated, Data: data})
	}
}
