package gluetun

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func writeServers(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	doc := `{"version":4,"servers":[
	  {"region":"CA Toronto","port_forward":true},
	  {"region":"CA Toronto","port_forward":true},
	  {"region":"US East","port_forward":false},
	  {"region":"Switzerland","port_forward":true}
	]}`
	if err := os.WriteFile(filepath.Join(dir, "private internet access.json"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRegionCatalog(t *testing.T) {
	rc := RegionCatalog{Dir: writeServers(t)}
	got, err := rc.Regions("private internet access", true)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"CA Toronto", "Switzerland"}; !slices.Equal(got, want) {
		t.Errorf("port-forward regions = %v, want %v (deduped, sorted, no US East)", got, want)
	}
	all, _ := rc.Regions("Private Internet Access", false)
	if len(all) != 3 {
		t.Errorf("all regions = %v, want 3", all)
	}
	if _, err := rc.Regions("../etc/passwd", false); err == nil {
		t.Error("a provider name with a path in it must be refused")
	}
	if _, err := (RegionCatalog{}).Regions("private internet access", false); err == nil {
		t.Error("no directory: no catalog")
	}
}

// fakeRegionClient extends the rotation fake with Gluetun's settings.
type fakeRegionClient struct {
	fakeRotateClient
	mu      sync.Mutex
	regions []string
	pf      bool
	sets    [][]string
	setErr  error
}

func (f *fakeRegionClient) VPNSettings(context.Context) (VPNSelection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return VPNSelection{Provider: "private internet access", Regions: f.regions, PortForwarding: f.pf}, nil
}

func (f *fakeRegionClient) SetRegions(_ context.Context, regions []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setErr != nil {
		return f.setErr
	}
	f.sets = append(f.sets, regions)
	f.regions = regions
	if f.idx < len(f.ips)-1 {
		f.idx++ // a new region is a new server
	}
	return nil
}

type memPref struct{ v string }

func (m *memPref) Get(context.Context) (string, error) { return m.v, nil }
func (m *memPref) Set(_ context.Context, v string) error {
	m.v = v
	return nil
}

func regionRotator(t *testing.T, client *fakeRegionClient, prefs *memPref, cleared *bool) *Rotator {
	t.Helper()
	var prewarmed bool
	r := newTestRotator(client, cleared, &prewarmed)
	r.EnableRegions(client, RegionCatalog{Dir: writeServers(t)}, prefs)
	return r
}

func TestChangeRegion_AppliesRemembersAndClearsState(t *testing.T) {
	client := &fakeRegionClient{fakeRotateClient: fakeRotateClient{ips: []string{"1.1.1.1", "9.9.9.9"}}, regions: []string{"CA Toronto"}, pf: true}
	prefs := &memPref{}
	var cleared bool
	r := regionRotator(t, client, prefs, &cleared)

	res, err := r.ChangeRegion(context.Background(), "Switzerland")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.NewIP != "9.9.9.9" || res.Region != "Switzerland" {
		t.Errorf("result = %+v", res)
	}
	if prefs.v != "Switzerland" {
		t.Errorf("preference = %q, want Switzerland (so it survives a VPN restart)", prefs.v)
	}
	if !cleared {
		t.Error("Cloudflare sessions belong to the old IP and must be cleared")
	}
}

func TestChangeRegion_RefusesRegionsItCantUse(t *testing.T) {
	client := &fakeRegionClient{fakeRotateClient: fakeRotateClient{ips: []string{"1.1.1.1"}}, pf: true}
	var cleared bool
	r := regionRotator(t, client, &memPref{}, &cleared)

	// US East has no port-forwarding server; with port forwarding on,
	// the torrent client would be unreachable there.
	if _, err := r.ChangeRegion(context.Background(), "US East"); err == nil {
		t.Error("changing to a region without port forwarding must be refused")
	}
	if len(client.sets) != 0 {
		t.Errorf("Gluetun was asked to change to %v", client.sets)
	}
}

func TestChangeRegion_SharesRotationCooldown(t *testing.T) {
	client := &fakeRegionClient{fakeRotateClient: fakeRotateClient{ips: []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}}, pf: true}
	var cleared bool
	r := regionRotator(t, client, &memPref{}, &cleared)

	if _, err := r.Rotate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ChangeRegion(context.Background(), "Switzerland"); !errors.Is(err, ErrRotateTooSoon) {
		t.Errorf("region change right after a rotation: err = %v, want ErrRotateTooSoon", err)
	}
}

func TestChangeRegion_UnavailableWithoutSetup(t *testing.T) {
	var cleared, prewarmed bool
	r := newTestRotator(&fakeRotateClient{ips: []string{"1.1.1.1"}}, &cleared, &prewarmed)
	if info := r.Regions(context.Background()); info.Available || info.Reason == "" {
		t.Errorf("regions = %+v, want unavailable with a reason", info)
	}
	if _, err := r.ChangeRegion(context.Background(), "Switzerland"); !errors.Is(err, ErrRegionsUnavailable) {
		t.Errorf("err = %v, want ErrRegionsUnavailable", err)
	}
}

// After the VPN container restarts, its environment's regions apply
// again; the chosen region must be put back.
func TestReapplyPreferred_AfterVPNRestart(t *testing.T) {
	client := &fakeRegionClient{fakeRotateClient: fakeRotateClient{ips: []string{"1.1.1.1", "9.9.9.9"}}, regions: []string{"CA Toronto"}, pf: true}
	prefs := &memPref{v: "Switzerland"}
	var cleared bool
	r := regionRotator(t, client, prefs, &cleared)

	r.reapplyPreferred(context.Background())
	if len(client.sets) != 1 || client.sets[0][0] != "Switzerland" {
		t.Fatalf("sets = %v, want Switzerland put back", client.sets)
	}
	// Already right: nothing to do.
	r.lastRotate = r.lastRotate.Add(-rotateMinInterval)
	r.reapplyPreferred(context.Background())
	if len(client.sets) != 1 {
		t.Errorf("sets = %v, want no change when Gluetun already uses the chosen region", client.sets)
	}
}

func TestReapplyPreferred_NothingChosen(t *testing.T) {
	client := &fakeRegionClient{fakeRotateClient: fakeRotateClient{ips: []string{"1.1.1.1"}}, regions: []string{"CA Toronto"}}
	var cleared bool
	r := regionRotator(t, client, &memPref{}, &cleared)
	r.reapplyPreferred(context.Background())
	if len(client.sets) != 0 {
		t.Errorf("sets = %v, want none when no region was chosen", client.sets)
	}
}

// The real client: settings read without keeping credentials, region
// PUT shape, and the API key header on every request.
func TestClient_VPNSettingsAndSetRegions(t *testing.T) {
	var putBody string
	var keys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("X-API-Key"))
		switch {
		case r.URL.Path == "/v1/vpn/settings" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"type":"openvpn","provider":{"name":"private internet access",
			  "server_selection":{"regions":["CA Toronto"]},"port_forwarding":{"enabled":true}},
			  "openvpn":{"user":"p1234","password":"secret"}}`))
		case r.URL.Path == "/v1/vpn/settings" && r.Method == http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			putBody = string(b)
			_, _ = w.Write([]byte("settings updated"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, slog.Default())
	c.SetAPIKey("k3y")
	sel, err := c.VPNSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sel.Provider != "private internet access" || !slices.Equal(sel.Regions, []string{"CA Toronto"}) || !sel.PortForwarding {
		t.Errorf("selection = %+v", sel)
	}
	if strings.Contains(strings.ToLower(jsonString(sel)), "secret") {
		t.Error("credentials leaked into the selection")
	}
	if err := c.SetRegions(context.Background(), []string{"Switzerland"}); err != nil {
		t.Fatal(err)
	}
	if putBody != `{"provider":{"server_selection":{"regions":["Switzerland"]}}}` {
		t.Errorf("PUT body = %s", putBody)
	}
	for _, k := range keys {
		if k != "k3y" {
			t.Errorf("request sent X-API-Key %q, want k3y", k)
		}
	}
}

func TestClient_SetRegionsSurfacesValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "region \"Atlantis\" is not valid", http.StatusBadRequest)
	}))
	defer srv.Close()
	err := NewClient(srv.URL, slog.Default()).SetRegions(context.Background(), []string{"Atlantis"})
	if err == nil || !strings.Contains(err.Error(), "Atlantis") {
		t.Errorf("err = %v, want Gluetun's validation message", err)
	}
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
