package v1

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	dbsqlite "github.com/arrsenal/configurarr/internal/db/generated/sqlite"
	"github.com/arrsenal/configurarr/internal/core/indexer"
)

// ── Request / Response types ─────────────────────────────────────────────────

type indexerBody struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Enabled   bool   `json:"enabled"`
	Priority  int    `json:"priority"`
	URL       string `json:"url"`
	Settings  string `json:"settings"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type indexerCreateInput struct {
	Body struct {
		Name     string `json:"name" required:"true" minLength:"1" maxLength:"200"`
		Kind     string `json:"kind,omitempty"`
		Enabled  *bool  `json:"enabled,omitempty"`
		Priority *int   `json:"priority,omitempty"`
		URL      string `json:"url" required:"true" minLength:"1"`
		APIKey   string `json:"api_key,omitempty"`
		Settings string `json:"settings,omitempty"`
	}
}

type indexerUpdateInput struct {
	ID   string `path:"id"`
	Body struct {
		Name     string `json:"name" required:"true" minLength:"1" maxLength:"200"`
		Kind     string `json:"kind,omitempty"`
		Enabled  *bool  `json:"enabled,omitempty"`
		Priority *int   `json:"priority,omitempty"`
		URL      string `json:"url" required:"true" minLength:"1"`
		APIKey   string `json:"api_key,omitempty"`
		Settings string `json:"settings,omitempty"`
	}
}

type indexerIDInput struct {
	ID string `path:"id"`
}

type assignInput struct {
	ID   string `path:"id"` // indexer ID
	Body struct {
		ServiceID string `json:"service_id" required:"true"`
		Overrides string `json:"overrides,omitempty"`
	}
}

type unassignInput struct {
	ID        string `path:"id"`         // indexer ID
	ServiceID string `path:"service_id"` // service ID
}

type indexerForServiceInput struct {
	ServiceID string `path:"service_id"`
}

func toIndexerBody(row dbsqlite.Indexer) indexerBody {
	return indexerBody{
		ID:        row.ID,
		Name:      row.Name,
		Kind:      row.Kind,
		Enabled:   row.Enabled == 1,
		Priority:  int(row.Priority),
		URL:       row.Url,
		Settings:  row.Settings,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// RegisterIndexerRoutes registers indexer management endpoints.
// proxyBaseURL is the Configurarr external URL used for Torznab proxy URL rewriting
// (e.g., "http://configurarr:9696"). If empty, URLs are not rewritten.
func RegisterIndexerRoutes(api huma.API, mgr *indexer.Manager, proxyBaseURL ...string) {
	baseURL := ""
	if len(proxyBaseURL) > 0 {
		baseURL = strings.TrimRight(proxyBaseURL[0], "/")
	}
	// POST /api/v1/indexers — create
	huma.Register(api, huma.Operation{
		OperationID:   "create-indexer",
		Method:        http.MethodPost,
		Path:          "/api/v1/indexers",
		Summary:       "Create a new indexer",
		Tags:          []string{"Indexers"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *indexerCreateInput) (*struct{ Body indexerBody }, error) {
		enabled := true
		if input.Body.Enabled != nil {
			enabled = *input.Body.Enabled
		}
		priority := 25
		if input.Body.Priority != nil {
			priority = *input.Body.Priority
		}

		row, err := mgr.Create(ctx, indexer.Input{
			Name:     input.Body.Name,
			Kind:     input.Body.Kind,
			Enabled:  enabled,
			Priority: priority,
			URL:      input.Body.URL,
			APIKey:   input.Body.APIKey,
			Settings: input.Body.Settings,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to create indexer", err)
		}
		return &struct{ Body indexerBody }{Body: toIndexerBody(*row)}, nil
	})

	// GET /api/v1/indexers — list all
	huma.Register(api, huma.Operation{
		OperationID: "list-indexers",
		Method:      http.MethodGet,
		Path:        "/api/v1/indexers",
		Summary:     "List all indexers",
		Tags:        []string{"Indexers"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []indexerBody }, error) {
		rows, err := mgr.List(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list indexers", err)
		}
		items := make([]indexerBody, len(rows))
		for i, r := range rows {
			items[i] = toIndexerBody(r)
		}
		return &struct{ Body []indexerBody }{Body: items}, nil
	})

	// GET /api/v1/indexers/{id}
	huma.Register(api, huma.Operation{
		OperationID: "get-indexer",
		Method:      http.MethodGet,
		Path:        "/api/v1/indexers/{id}",
		Summary:     "Get an indexer by ID",
		Tags:        []string{"Indexers"},
	}, func(ctx context.Context, input *indexerIDInput) (*struct{ Body indexerBody }, error) {
		row, err := mgr.Get(ctx, input.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusNotFound, "indexer not found", err)
		}
		return &struct{ Body indexerBody }{Body: toIndexerBody(*row)}, nil
	})

	// PUT /api/v1/indexers/{id} — update
	huma.Register(api, huma.Operation{
		OperationID: "update-indexer",
		Method:      http.MethodPut,
		Path:        "/api/v1/indexers/{id}",
		Summary:     "Update an indexer",
		Tags:        []string{"Indexers"},
	}, func(ctx context.Context, input *indexerUpdateInput) (*struct{ Body indexerBody }, error) {
		enabled := true
		if input.Body.Enabled != nil {
			enabled = *input.Body.Enabled
		}
		priority := 25
		if input.Body.Priority != nil {
			priority = *input.Body.Priority
		}

		row, err := mgr.Update(ctx, input.ID, indexer.Input{
			Name:     input.Body.Name,
			Kind:     input.Body.Kind,
			Enabled:  enabled,
			Priority: priority,
			URL:      input.Body.URL,
			APIKey:   input.Body.APIKey,
			Settings: input.Body.Settings,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to update indexer", err)
		}
		return &struct{ Body indexerBody }{Body: toIndexerBody(*row)}, nil
	})

	// DELETE /api/v1/indexers/{id}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-indexer",
		Method:        http.MethodDelete,
		Path:          "/api/v1/indexers/{id}",
		Summary:       "Delete an indexer",
		Tags:          []string{"Indexers"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *indexerIDInput) (*struct{}, error) {
		if err := mgr.Delete(ctx, input.ID); err != nil {
			return nil, huma.NewError(http.StatusNotFound, "indexer not found", err)
		}
		return nil, nil
	})

	// POST /api/v1/indexers/{id}/assign — assign to service
	huma.Register(api, huma.Operation{
		OperationID:   "assign-indexer",
		Method:        http.MethodPost,
		Path:          "/api/v1/indexers/{id}/assign",
		Summary:       "Assign an indexer to a service",
		Tags:          []string{"Indexers"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *assignInput) (*struct{ Body dbsqlite.IndexerAssignment }, error) {
		row, err := mgr.Assign(ctx, indexer.AssignmentInput{
			IndexerID: input.ID,
			ServiceID: input.Body.ServiceID,
			Overrides: input.Body.Overrides,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusConflict, "assignment failed", err)
		}
		return &struct{ Body dbsqlite.IndexerAssignment }{Body: *row}, nil
	})

	// DELETE /api/v1/indexers/{id}/assign/{service_id} — unassign
	huma.Register(api, huma.Operation{
		OperationID:   "unassign-indexer",
		Method:        http.MethodDelete,
		Path:          "/api/v1/indexers/{id}/assign/{service_id}",
		Summary:       "Unassign an indexer from a service",
		Tags:          []string{"Indexers"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *unassignInput) (*struct{}, error) {
		if err := mgr.Unassign(ctx, input.ID, input.ServiceID); err != nil {
			return nil, huma.NewError(http.StatusNotFound, "assignment not found", err)
		}
		return nil, nil
	})

	// GET /api/v1/indexers/{id}/assignments — list assignments
	huma.Register(api, huma.Operation{
		OperationID: "list-indexer-assignments",
		Method:      http.MethodGet,
		Path:        "/api/v1/indexers/{id}/assignments",
		Summary:     "List services assigned to an indexer",
		Tags:        []string{"Indexers"},
	}, func(ctx context.Context, input *indexerIDInput) (*struct{ Body []dbsqlite.IndexerAssignment }, error) {
		rows, err := mgr.ListAssignments(ctx, input.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list assignments", err)
		}
		return &struct{ Body []dbsqlite.IndexerAssignment }{Body: rows}, nil
	})

	// POST /api/v1/indexers/test — test indexer connectivity
	huma.Register(api, huma.Operation{
		OperationID: "test-indexer",
		Method:      http.MethodPost,
		Path:        "/api/v1/indexers/test",
		Summary:     "Test indexer connectivity and credentials",
		Tags:        []string{"Indexers"},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Kind   string `json:"kind" required:"true"`
			URL    string `json:"url" required:"true"`
			APIKey string `json:"api_key,omitempty"`
		}
	}) (*struct{ Body indexer.TestResult }, error) {
		result := indexer.TestIndexer(ctx, input.Body.Kind, input.Body.URL, input.Body.APIKey)
		return &struct{ Body indexer.TestResult }{Body: result}, nil
	})

	// GET /api/v1/services/{service_id}/indexers — list indexers for a service
	huma.Register(api, huma.Operation{
		OperationID: "list-indexers-for-service",
		Method:      http.MethodGet,
		Path:        "/api/v1/services/{service_id}/indexers",
		Summary:     "List indexers assigned to a service",
		Tags:        []string{"Indexers"},
	}, func(ctx context.Context, input *indexerForServiceInput) (*struct{ Body []indexerBody }, error) {
		rows, err := mgr.ListForService(ctx, input.ServiceID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list indexers", err)
		}
		items := make([]indexerBody, len(rows))
		for i, r := range rows {
			body := toIndexerBody(r)
			// Rewrite URL to point at Configurarr's Torznab proxy for scraped indexers.
			// If the URL isn't already a Torznab/Newznab API endpoint, replace it
			// with the proxy URL so Luminarr queries through Configurarr.
			if baseURL != "" && !isNativeTorznabURL(r.Url) {
				body.URL = fmt.Sprintf("%s/api/v1/torznab/%s", baseURL, r.ID)
			}
			items[i] = body
		}
		return &struct{ Body []indexerBody }{Body: items}, nil
	})
}

// isNativeTorznabURL returns true if the URL looks like an existing Torznab/Newznab
// API endpoint (e.g., from Jackett or Prowlarr). In that case, don't rewrite it.
func isNativeTorznabURL(u string) bool {
	lower := strings.ToLower(u)
	return strings.Contains(lower, "/api") ||
		strings.Contains(lower, "torznab") ||
		strings.Contains(lower, "newznab") ||
		strings.Contains(lower, "?t=")
}
