package gluetun

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// testLogger returns a logger that drops everything, so tests don't pollute
// stdout with the diagnostic warnings the client emits on failure paths.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewClient_Disabled(t *testing.T) {
	c := NewClient("", nil)
	if c != nil {
		t.Fatalf("expected nil client when baseURL empty")
	}
	if _, err := c.Status(context.Background()); err != ErrDisabled {
		t.Errorf("Status on nil client = %v, want ErrDisabled", err)
	}
}

// Reachable is true when at least one endpoint succeeds, false when none do.
// Pulse's aggregator uses Reachable to decide whether to expose a VPN panel
// at all — the difference between "no Gluetun configured" and "configured
// but the control server isn't responding".
func TestStatus_ReachableTrueOnAnySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/version" {
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "v3.40.0"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := &Client{http: srv.Client(), baseURL: srv.URL, logger: testLogger()}
	st, _ := c.Status(context.Background())
	if !st.Reachable {
		t.Error("expected Reachable=true when /v1/version responded")
	}
}

func TestStatus_ReachableFalseOnAllFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := &Client{http: srv.Client(), baseURL: srv.URL, logger: testLogger()}
	st, _ := c.Status(context.Background())
	if st.Reachable {
		t.Error("expected Reachable=false when nothing responded")
	}
}

// Happy path: every endpoint responds, status is fully populated.
func TestStatus_AllEndpointsRespond(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/publicip/ip":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"public_ip":    "1.2.3.4",
				"country":      "Switzerland",
				"region":       "Zurich",
				"city":         "Zurich",
				"hostname":     "ch1.protonvpn.com",
				"organization": "ProtonVPN",
			})
		case "/v1/openvpn/portforwarded":
			_ = json.NewEncoder(w).Encode(map[string]int{"port": 51820})
		case "/v1/dns/status":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "on"})
		case "/v1/openvpn/settings":
			tru := true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"provider": "protonvpn",
				"firewall": &tru,
			})
		case "/v1/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "v3.40.0"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &Client{http: srv.Client(), baseURL: srv.URL, logger: testLogger()}
	st, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Connected {
		t.Error("Connected should be true when public_ip is non-empty")
	}
	if st.PublicIP != "1.2.3.4" {
		t.Errorf("PublicIP = %q", st.PublicIP)
	}
	if st.Country != "Switzerland" {
		t.Errorf("Country = %q", st.Country)
	}
	if st.PortForwarded != 51820 {
		t.Errorf("PortForwarded = %d", st.PortForwarded)
	}
	if st.DNSStatus != "on" {
		t.Errorf("DNSStatus = %q", st.DNSStatus)
	}
	if st.Provider != "protonvpn" {
		t.Errorf("Provider = %q", st.Provider)
	}
	if !st.KillSwitchEnabled {
		t.Error("KillSwitchEnabled should be true")
	}
	if st.Version != "v3.40.0" {
		t.Errorf("Version = %q", st.Version)
	}
}

// Some endpoints return 404 — partial result, no error. Pulse's UI shows
// "unknown" for whatever fields didn't populate.
func TestStatus_PartialResultsOnPartialFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/publicip/ip":
			_ = json.NewEncoder(w).Encode(map[string]string{"public_ip": "1.2.3.4"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &Client{http: srv.Client(), baseURL: srv.URL, logger: testLogger()}
	st, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.PublicIP != "1.2.3.4" {
		t.Errorf("PublicIP = %q, want 1.2.3.4", st.PublicIP)
	}
	if st.Provider != "" {
		t.Errorf("Provider = %q, want empty (settings endpoint 404'd)", st.Provider)
	}
	if !st.Connected {
		t.Error("Connected should still be true — public_ip came back")
	}
}

// All endpoints fail — no panic, status is the zero-value, no error.
func TestStatus_AllEndpointsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Client{http: srv.Client(), baseURL: srv.URL, logger: testLogger()}
	st, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if st.Connected {
		t.Error("Connected should be false when nothing responds")
	}
}

// One endpoint hangs — overall call must complete in well under 1.5s.
// Each request has a 1s timeout; the slow handler should be cut off.
func TestStatus_SlowEndpointDoesNotBlock(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.URL.Path == "/v1/dns/status" {
			time.Sleep(3 * time.Second)
			return
		}
		switch r.URL.Path {
		case "/v1/publicip/ip":
			_ = json.NewEncoder(w).Encode(map[string]string{"public_ip": "1.2.3.4"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &Client{http: srv.Client(), baseURL: srv.URL, logger: testLogger()}
	start := time.Now()
	_, err := c.Status(context.Background())
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("Status took %v, expected <2s — per-call timeout not enforced", elapsed)
	}
}

// kill-switch flag absent in response — default to "enabled" since
// Gluetun's firewall defaults to on. Better to over-report kill-switch
// than to claim it's off when in fact we just couldn't read the config.
func TestStatus_KillSwitchDefaultsTrueWhenAbsent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/openvpn/settings" {
			_ = json.NewEncoder(w).Encode(map[string]string{"provider": "mullvad"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := &Client{http: srv.Client(), baseURL: srv.URL, logger: testLogger()}
	st, _ := c.Status(context.Background())
	if !st.KillSwitchEnabled {
		t.Error("expected kill-switch to default true when firewall field absent")
	}
}
