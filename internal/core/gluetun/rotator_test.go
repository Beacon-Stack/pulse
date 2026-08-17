package gluetun

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newControlServer stands in for gluetun's control server, recording the
// last /v1/openvpn/status request and serving a fixed public IP.
func newControlServer(t *testing.T, gotMethod, gotBody *string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/openvpn/status":
			b, _ := io.ReadAll(r.Body)
			*gotMethod, *gotBody = r.Method, string(b)
			_, _ = w.Write([]byte(`{"outcome":"success"}`))
		case "/v1/publicip/ip":
			_, _ = w.Write([]byte(`{"public_ip":"5.6.7.8"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// fakeRotateClient scripts a sequence of public IPs: index 0 is the
// pre-rotation IP; each tunnel cycle advances to the next entry.
type fakeRotateClient struct {
	ips      []string
	idx      int
	statuses []string // recorded SetOpenVPNStatus calls
	putErr   error
}

func (f *fakeRotateClient) PublicIP(_ context.Context) (string, error) {
	i := f.idx
	if i >= len(f.ips) {
		i = len(f.ips) - 1
	}
	return f.ips[i], nil
}

func (f *fakeRotateClient) SetOpenVPNStatus(_ context.Context, status string) error {
	f.statuses = append(f.statuses, status)
	if f.putErr != nil {
		return f.putErr
	}
	if status == "running" && f.idx < len(f.ips)-1 {
		f.idx++
	}
	return nil
}

func newTestRotator(client rotateClient, cleared *bool, prewarmed *bool) *Rotator {
	r := NewRotator(client,
		func() { *cleared = true },
		func(context.Context) { *prewarmed = true },
		nil, slog.Default())
	r.pollInterval = time.Millisecond
	r.pollBudget = 100 * time.Millisecond
	return r
}

func TestRotate_ChangesIPFirstAttempt(t *testing.T) {
	client := &fakeRotateClient{ips: []string{"1.1.1.1", "2.2.2.2"}}
	var cleared, prewarmed bool
	r := newTestRotator(client, &cleared, &prewarmed)

	res, err := r.Rotate(context.Background())
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if !res.Changed || res.OldIP != "1.1.1.1" || res.NewIP != "2.2.2.2" || res.Attempts != 1 {
		t.Errorf("result = %+v, want changed 1.1.1.1→2.2.2.2 in 1 attempt", res)
	}
	if !cleared {
		t.Error("scraper state was not cleared — stale CF sessions would poison searches")
	}
	if got := client.statuses; len(got) != 2 || got[0] != "stopped" || got[1] != "running" {
		t.Errorf("status calls = %v, want [stopped running]", got)
	}
}

// The provider can reconnect to the same server (observed live with PIA
// Toronto) — the rotator must retry, and after exhausting attempts
// report Changed=false WITHOUT an error: the tunnel is healthy.
func TestRotate_SameIPRetriesAndReportsHonestly(t *testing.T) {
	client := &fakeRotateClient{ips: []string{"1.1.1.1"}} // never changes
	var cleared, prewarmed bool
	r := newTestRotator(client, &cleared, &prewarmed)

	res, err := r.Rotate(context.Background())
	if err != nil {
		t.Fatalf("Rotate: %v (same-IP outcome must not be an error)", err)
	}
	if res.Changed || res.Attempts != rotateMaxAttempts {
		t.Errorf("result = %+v, want unchanged after %d attempts", res, rotateMaxAttempts)
	}
	if !cleared {
		t.Error("sessions must be cleared even when the IP didn't change — the tunnel still bounced")
	}
}

func TestRotate_RateLimited(t *testing.T) {
	client := &fakeRotateClient{ips: []string{"1.1.1.1", "2.2.2.2"}}
	var cleared, prewarmed bool
	r := newTestRotator(client, &cleared, &prewarmed)

	if _, err := r.Rotate(context.Background()); err != nil {
		t.Fatalf("first Rotate: %v", err)
	}
	_, err := r.Rotate(context.Background())
	if !errors.Is(err, ErrRotateTooSoon) {
		t.Fatalf("second immediate Rotate error = %v, want ErrRotateTooSoon", err)
	}
}

func TestClient_SetOpenVPNStatusAndPublicIP(t *testing.T) {
	var gotMethod, gotBody string
	srv := newControlServer(t, &gotMethod, &gotBody)

	c := NewClient(srv, slog.Default())
	if err := c.SetOpenVPNStatus(context.Background(), "stopped"); err != nil {
		t.Fatalf("SetOpenVPNStatus: %v", err)
	}
	if gotMethod != "PUT" || gotBody != `{"status":"stopped"}` {
		t.Errorf("control server saw %s %s, want PUT {\"status\":\"stopped\"}", gotMethod, gotBody)
	}

	ip, err := c.PublicIP(context.Background())
	if err != nil {
		t.Fatalf("PublicIP: %v", err)
	}
	if ip != "5.6.7.8" {
		t.Errorf("PublicIP = %q, want 5.6.7.8", ip)
	}
}

func TestRotate_TunnelControlFailureSurfaces(t *testing.T) {
	client := &fakeRotateClient{ips: []string{"1.1.1.1"}, putErr: errors.New("control server down")}
	var cleared, prewarmed bool
	r := newTestRotator(client, &cleared, &prewarmed)

	if _, err := r.Rotate(context.Background()); err == nil {
		t.Fatal("Rotate should surface a tunnel-control failure")
	}
}
