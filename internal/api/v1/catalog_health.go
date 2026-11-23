package v1

import (
	"context"
	"errors"
	"net/http"
	"sort"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pulse/internal/core/indexerprobe"
)

type catalogHealthOutput struct {
	Body struct {
		Status indexerprobe.Status        `json:"status"`
		Sites  []*indexerprobe.SiteHealth `json:"sites"`
		// Eligible lists the catalog IDs the background tester covers.
		Eligible []string `json:"eligible"`
		// Added lists catalog IDs that are already added; their stats come
		// from real searches (GET /api/v1/indexers/stats) instead.
		Added []string `json:"added"`
	}
}

type catalogTestInput struct {
	ID string `path:"catalog_id"`
}

// RegisterCatalogHealthRoutes exposes the catalog tester's results.
func RegisterCatalogHealthRoutes(api huma.API, prober *indexerprobe.Prober) {
	huma.Register(api, huma.Operation{
		OperationID: "catalog-health",
		Method:      http.MethodGet,
		Path:        "/api/v1/indexers/catalog/health",
		Summary:     "Test results for catalog indexers that haven't been added",
		Description: "Public indexers in the categories your services use are test-searched in the background every 6 hours. Each tested site gets a verdict and a score for ranking.",
		Tags:        []string{"Catalog"},
	}, func(ctx context.Context, _ *struct{}) (*catalogHealthOutput, error) {
		health, err := prober.Health(ctx)
		if err != nil {
			return nil, huma.Error500InternalServerError("loading test results", err)
		}
		targets, _, added, err := prober.Eligible(ctx)
		if err != nil {
			return nil, huma.Error500InternalServerError("listing indexers", err)
		}
		out := &catalogHealthOutput{}
		out.Body.Status = prober.Status()
		out.Body.Sites = make([]*indexerprobe.SiteHealth, 0, len(health))
		for id, h := range health {
			if !added[id] {
				out.Body.Sites = append(out.Body.Sites, h)
			}
		}
		sort.Slice(out.Body.Sites, func(i, j int) bool { return out.Body.Sites[i].CatalogID < out.Body.Sites[j].CatalogID })
		out.Body.Eligible = make([]string, 0, len(targets))
		for _, t := range targets {
			out.Body.Eligible = append(out.Body.Eligible, t.ID)
		}
		out.Body.Added = make([]string, 0, len(added))
		for id := range added {
			out.Body.Added = append(out.Body.Added, id)
		}
		sort.Strings(out.Body.Added)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "catalog-test-now",
		Method:        http.MethodPost,
		Path:          "/api/v1/indexers/catalog/{catalog_id}/test",
		Summary:       "Test a catalog indexer now",
		Description:   "Runs one test search against a public catalog indexer and records the result. At most once per 10 minutes per indexer. Cloudflare sites can take a minute or two.",
		Tags:          []string{"Catalog"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *catalogTestInput) (*struct{}, error) {
		err := prober.TestNow(ctx, in.ID)
		switch {
		case errors.Is(err, indexerprobe.ErrCooldown):
			return nil, huma.Error429TooManyRequests("This indexer was tested less than 10 minutes ago. Try again shortly.")
		case err != nil:
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, nil
	})
}
