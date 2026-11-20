package v1

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pulse/internal/core/gluetun"
	"github.com/beacon-stack/pulse/internal/scraper"
)

type rotateVPNBody struct {
	OldIP    string `json:"old_ip"`
	NewIP    string `json:"new_ip"`
	Attempts int    `json:"attempts"`
	Changed  bool   `json:"changed"`
	Message  string `json:"message,omitempty"`
}

type rotateVPNOutput struct {
	Body rotateVPNBody
}

type vpnAdviceBody struct {
	// RecommendRotate: Cloudflare keeps refusing FlareSolverr on at least
	// one site from this IP, which a new IP or region usually fixes.
	RecommendRotate bool                  `json:"recommend_rotate"`
	Sites           []scraper.SiteTrouble `json:"sites"`
	WindowMinutes   int                   `json:"window_minutes"`
}

type vpnAdviceOutput struct {
	Body vpnAdviceBody
}

type vpnRegionsOutput struct {
	Body gluetun.RegionInfo
}

type changeRegionInput struct {
	Body struct {
		Region string `json:"region" minLength:"1" doc:"A region name from GET /api/v1/vpn/regions"`
	}
}

type changeRegionOutput struct {
	Body gluetun.RegionResult
}

// adviceWindowMinutes mirrors the scraper's challenge window, for the UI's
// "in the last N minutes" wording.
const adviceWindowMinutes = 120

// RegisterVPNRoutes registers the VPN control endpoints. Only called when
// a gluetun rotator is configured (see router.go's nil guard). troubles
// may be nil (no FlareSolverr); advice then never recommends rotating.
func RegisterVPNRoutes(api huma.API, rotator *gluetun.Rotator, troubles func() []scraper.SiteTrouble) {
	// GET /api/v1/vpn/advice — should the user rotate?
	huma.Register(api, huma.Operation{
		OperationID: "vpn-advice",
		Method:      http.MethodGet,
		Path:        "/api/v1/vpn/advice",
		Summary:     "Sites whose Cloudflare challenges keep failing from this VPN IP",
		Tags:        []string{"VPN"},
	}, func(_ context.Context, _ *struct{}) (*vpnAdviceOutput, error) {
		body := vpnAdviceBody{Sites: []scraper.SiteTrouble{}, WindowMinutes: adviceWindowMinutes}
		if troubles != nil {
			if sites := troubles(); len(sites) > 0 {
				body.Sites = sites
				body.RecommendRotate = true
			}
		}
		return &vpnAdviceOutput{Body: body}, nil
	})

	// GET /api/v1/vpn/regions — current, chosen and available regions.
	huma.Register(api, huma.Operation{
		OperationID: "vpn-regions",
		Method:      http.MethodGet,
		Path:        "/api/v1/vpn/regions",
		Summary:     "VPN regions: current, chosen, and the ones this VPN can use",
		Tags:        []string{"VPN"},
	}, func(ctx context.Context, _ *struct{}) (*vpnRegionsOutput, error) {
		return &vpnRegionsOutput{Body: rotator.Regions(ctx)}, nil
	})

	// POST /api/v1/vpn/region — move the tunnel to another region.
	huma.Register(api, huma.Operation{
		OperationID: "change-vpn-region",
		Method:      http.MethodPost,
		Path:        "/api/v1/vpn/region",
		Summary:     "Change the VPN region",
		Description: "Reconnects the tunnel in the given region and remembers it, putting it back if the VPN container restarts. Clears IP-bound Cloudflare sessions and cached search results. Torrent traffic pauses briefly. Shares rotation's 5-minute cool-down.",
		Tags:        []string{"VPN"},
	}, func(ctx context.Context, in *changeRegionInput) (*changeRegionOutput, error) {
		res, err := rotator.ChangeRegion(ctx, in.Body.Region)
		switch {
		case err == nil:
			return &changeRegionOutput{Body: res}, nil
		case errors.Is(err, gluetun.ErrRotateTooSoon):
			return nil, huma.NewError(http.StatusTooManyRequests, err.Error())
		case errors.Is(err, gluetun.ErrRegionsUnavailable):
			return nil, huma.NewError(http.StatusServiceUnavailable, err.Error())
		case res.OldIP == "" && res.NewIP == "" && res.Region == "":
			// Refused before touching the tunnel (unknown region).
			return nil, huma.NewError(http.StatusBadRequest, err.Error())
		default:
			return nil, huma.NewError(http.StatusBadGateway, "VPN region change failed: "+err.Error())
		}
	})

	// POST /api/v1/vpn/rotate — cycle the tunnel until the exit IP changes.
	huma.Register(api, huma.Operation{
		OperationID: "rotate-vpn-ip",
		Method:      http.MethodPost,
		Path:        "/api/v1/vpn/rotate",
		Summary:     "Rotate the VPN exit IP",
		Description: "Cycles the Gluetun tunnel until the provider assigns a new exit IP (up to 3 attempts), then clears IP-bound Cloudflare sessions and cached search results. Torrent traffic pauses briefly during each cycle.",
		Tags:        []string{"VPN"},
	}, func(ctx context.Context, _ *struct{}) (*rotateVPNOutput, error) {
		res, err := rotator.Rotate(ctx)
		if err != nil {
			if errors.Is(err, gluetun.ErrRotateTooSoon) {
				return nil, huma.NewError(http.StatusTooManyRequests, err.Error())
			}
			return nil, huma.NewError(http.StatusBadGateway, "VPN rotation failed: "+err.Error())
		}
		out := &rotateVPNOutput{Body: rotateVPNBody{
			OldIP:    res.OldIP,
			NewIP:    res.NewIP,
			Attempts: res.Attempts,
			Changed:  res.Changed,
		}}
		if !res.Changed {
			out.Body.Message = "The provider kept assigning the same server — consider changing SERVER_REGIONS in the VPN config."
		}
		return out, nil
	})
}
