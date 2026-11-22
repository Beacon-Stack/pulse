package v1

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pulse/internal/version"
)

// systemStatusBody has the same fields as the other Beacon apps'
// /system/status, plus Pulse's older status and uptime strings.
type systemStatusBody struct {
	AppName       string `json:"app_name"`
	Version       string `json:"version"`
	BuildTime     string `json:"build_time"`
	GoVersion     string `json:"go_version"`
	DBType        string `json:"db_type"`
	DBPath        string `json:"db_path,omitempty"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	StartTime     string `json:"start_time"`
	Status        string `json:"status"`
	Uptime        string `json:"uptime"`
}

// RegisterSystemRoutes registers system/status endpoints.
func RegisterSystemRoutes(api huma.API, startTime time.Time, dbPath string) {
	huma.Register(api, huma.Operation{
		OperationID: "system-status",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/status",
		Summary:     "System status and uptime",
		Tags:        []string{"System"},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body systemStatusBody }, error) {
		return &struct{ Body systemStatusBody }{Body: systemStatusBody{
			AppName:       "Pulse",
			Version:       version.Version,
			BuildTime:     version.BuildTime,
			GoVersion:     runtime.Version(),
			DBType:        "sqlite",
			DBPath:        dbPath,
			UptimeSeconds: int64(time.Since(startTime).Seconds()),
			StartTime:     startTime.UTC().Format(time.RFC3339),
			Status:        "ok",
			Uptime:        time.Since(startTime).Truncate(time.Second).String(),
		}}, nil
	})
}
