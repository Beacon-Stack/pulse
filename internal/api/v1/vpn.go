package v1

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pulse/internal/core/gluetun"
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

// RegisterVPNRoutes registers the VPN control endpoints. Only called when
// a gluetun rotator is configured (see router.go's nil guard).
func RegisterVPNRoutes(api huma.API, rotator *gluetun.Rotator) {
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
