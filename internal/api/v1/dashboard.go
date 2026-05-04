package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pulse/internal/core/dashboard"
)

// RegisterDashboardRoutes registers the unified dashboard endpoints.
// The frontend polls /overview every 2s for the top-level cards and only
// hits /services/{id} when the drill-down drawer is open.
func RegisterDashboardRoutes(api huma.API, agg *dashboard.Aggregator) {
	huma.Register(api, huma.Operation{
		OperationID: "dashboard-overview",
		Method:      http.MethodGet,
		Path:        "/api/v1/dashboard/overview",
		Summary:     "Aggregated dashboard overview",
		Description: "Per-service container resource usage, Haul throughput, and VPN status. Polled every 2s by the dashboard.",
		Tags:        []string{"Dashboard"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body *dashboard.Overview }, error) {
		ov, err := agg.Overview(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "dashboard overview failed", err)
		}
		return &struct{ Body *dashboard.Overview }{Body: ov}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "dashboard-service-detail",
		Method:      http.MethodGet,
		Path:        "/api/v1/dashboard/services/{id}",
		Summary:     "Detailed view of a single service",
		Description: "Full container stats, Go-runtime stats, and a service-type-specific payload (Haul torrents, Pilot/Prism queue).",
		Tags:        []string{"Dashboard"},
	}, func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*struct{ Body *dashboard.ServiceDetail }, error) {
		detail, err := agg.ServiceDetail(ctx, input.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusNotFound, "service not found", err)
		}
		return &struct{ Body *dashboard.ServiceDetail }{Body: detail}, nil
	})
}
