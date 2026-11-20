package v1

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/beacon-stack/pulse/internal/core/gluetun"
	"github.com/beacon-stack/pulse/internal/scraper"
)

type staticIPClient struct{}

func (staticIPClient) PublicIP(context.Context) (string, error)       { return "1.1.1.1", nil }
func (staticIPClient) SetOpenVPNStatus(context.Context, string) error { return nil }

func vpnAPI(t *testing.T, troubles func() []scraper.SiteTrouble) humatest.TestAPI {
	t.Helper()
	_, api := humatest.New(t)
	rot := gluetun.NewRotator(staticIPClient{}, nil, nil, nil, slog.Default())
	RegisterVPNRoutes(api, rot, troubles)
	return api
}

func TestVPNAdvice_RecommendsRotateWhenSitesKeepFailing(t *testing.T) {
	api := vpnAPI(t, func() []scraper.SiteTrouble {
		return []scraper.SiteTrouble{{Host: "1337x.to", Failures: 6, LastFailure: time.Now()}}
	})
	resp := api.Get("/api/v1/vpn/advice")
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d: %s", resp.Code, resp.Body.String())
	}
	var body vpnAdviceBody
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.RecommendRotate || len(body.Sites) != 1 || body.Sites[0].Host != "1337x.to" {
		t.Errorf("advice = %+v", body)
	}
}

func TestVPNAdvice_QuietWithoutTroubleOrFlareSolverr(t *testing.T) {
	for name, troubles := range map[string]func() []scraper.SiteTrouble{
		"no trouble":      func() []scraper.SiteTrouble { return nil },
		"no flaresolverr": nil,
	} {
		resp := vpnAPI(t, troubles).Get("/api/v1/vpn/advice")
		var body vpnAdviceBody
		_ = json.Unmarshal(resp.Body.Bytes(), &body)
		if body.RecommendRotate || body.Sites == nil {
			t.Errorf("%s: advice = %+v, want no recommendation and an empty (not null) site list", name, body)
		}
	}
}

func TestVPNRegions_UnavailableExplainsWhy(t *testing.T) {
	resp := vpnAPI(t, nil).Get("/api/v1/vpn/regions")
	var info gluetun.RegionInfo
	_ = json.Unmarshal(resp.Body.Bytes(), &info)
	if info.Available || info.Reason == "" {
		t.Errorf("regions = %+v, want unavailable with a reason", info)
	}
	post := vpnAPI(t, nil).Post("/api/v1/vpn/region", map[string]string{"region": "Switzerland"})
	if post.Code != http.StatusServiceUnavailable {
		t.Errorf("POST region without setup: status %d, want 503", post.Code)
	}
}
