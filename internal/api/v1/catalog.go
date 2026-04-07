package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pulse/internal/core/indexer"
)

type catalogEntry struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Language    string          `json:"language"`
	Protocol    string          `json:"protocol"`
	Privacy     string          `json:"privacy"`
	Categories  []string        `json:"categories"`
	URLs        []string        `json:"urls"`
	Settings    []indexer.Field `json:"settings"`
}

type catalogResponse struct {
	Entries []catalogEntry `json:"entries"`
	Total   int            `json:"total"`
}

type catalogQueryInput struct {
	Query    string `query:"q"`
	Protocol string `query:"protocol"`
	Privacy  string `query:"privacy"`
	Category string `query:"category"`
	Language string `query:"language"`
}

func toCatalogEntry(e indexer.CatalogEntry) catalogEntry {
	urls := e.URLs
	if urls == nil {
		urls = []string{}
	}
	settings := e.Settings
	if settings == nil {
		settings = []indexer.Field{}
	}
	return catalogEntry{
		ID:          e.ID,
		Name:        e.Name,
		Description: e.Description,
		Language:    e.Language,
		Protocol:    e.Protocol,
		Privacy:     e.Privacy,
		Categories:  e.Categories,
		URLs:        urls,
		Settings:    settings,
	}
}

// RegisterCatalogRoutes registers the indexer catalog browsing endpoints.
func RegisterCatalogRoutes(api huma.API) {
	// GET /api/v1/indexers/catalog — browse available indexers
	huma.Register(api, huma.Operation{
		OperationID: "browse-indexer-catalog",
		Method:      http.MethodGet,
		Path:        "/api/v1/indexers/catalog",
		Summary:     "Browse the indexer catalog with search and filters",
		Tags:        []string{"Catalog"},
	}, func(_ context.Context, input *catalogQueryInput) (*struct{ Body catalogResponse }, error) {
		entries := indexer.FilterCatalog(indexer.CatalogFilter{
			Query:    input.Query,
			Protocol: input.Protocol,
			Privacy:  input.Privacy,
			Category: input.Category,
			Language: input.Language,
		})

		items := make([]catalogEntry, len(entries))
		for i, e := range entries {
			items[i] = toCatalogEntry(e)
		}

		return &struct{ Body catalogResponse }{Body: catalogResponse{
			Entries: items,
			Total:   len(items),
		}}, nil
	})

	// GET /api/v1/indexers/catalog/languages — list available languages
	huma.Register(api, huma.Operation{
		OperationID: "list-catalog-languages",
		Method:      http.MethodGet,
		Path:        "/api/v1/indexers/catalog/languages",
		Summary:     "List distinct languages in the indexer catalog",
		Tags:        []string{"Catalog"},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body []string }, error) {
		return &struct{ Body []string }{Body: indexer.CatalogLanguages()}, nil
	})

	// GET /api/v1/indexers/catalog/{id} — get a single catalog entry
	huma.Register(api, huma.Operation{
		OperationID: "get-catalog-entry",
		Method:      http.MethodGet,
		Path:        "/api/v1/indexers/catalog/{id}",
		Summary:     "Get a single indexer catalog entry by ID",
		Tags:        []string{"Catalog"},
	}, func(_ context.Context, input *struct {
		ID string `path:"id"`
	}) (*struct{ Body catalogEntry }, error) {
		for _, e := range indexer.Catalog() {
			if e.ID == input.ID {
				return &struct{ Body catalogEntry }{Body: toCatalogEntry(e)}, nil
			}
		}
		return nil, huma.NewError(http.StatusNotFound, "catalog entry not found")
	})
}
