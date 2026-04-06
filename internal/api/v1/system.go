package v1

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

type systemStatusBody struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

// RegisterSystemRoutes registers system/status endpoints.
func RegisterSystemRoutes(api huma.API, startTime time.Time) {
	huma.Register(api, huma.Operation{
		OperationID: "system-status",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/status",
		Summary:     "System status and uptime",
		Tags:        []string{"System"},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body systemStatusBody }, error) {
		return &struct{ Body systemStatusBody }{Body: systemStatusBody{
			Status:  "ok",
			Version: "0.1.0",
			Uptime:  time.Since(startTime).Truncate(time.Second).String(),
		}}, nil
	})
}
