package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/arrsenal/configurarr/internal/core/tag"
)

type tagBody struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ServiceCount int64  `json:"service_count"`
	IndexerCount int64  `json:"indexer_count"`
}

type tagCreateInput struct {
	Body struct {
		Name string `json:"name" required:"true" minLength:"1" maxLength:"100"`
	}
}

type tagUpdateInput struct {
	ID   string `path:"id"`
	Body struct {
		Name string `json:"name" required:"true" minLength:"1" maxLength:"100"`
	}
}

type tagDeleteInput struct {
	ID string `path:"id"`
}

// RegisterTagRoutes registers the tag management endpoints.
func RegisterTagRoutes(api huma.API, svc *tag.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "list-tags",
		Method:      http.MethodGet,
		Path:        "/api/v1/tags",
		Summary:     "List all tags with usage counts",
		Tags:        []string{"Tags"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []tagBody }, error) {
		tags, err := svc.List(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list tags", err)
		}
		items := make([]tagBody, len(tags))
		for i, t := range tags {
			items[i] = tagBody{
				ID:           t.ID,
				Name:         t.Name,
				ServiceCount: t.ServiceCount,
				IndexerCount: t.IndexerCount,
			}
		}
		return &struct{ Body []tagBody }{Body: items}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-tag",
		Method:        http.MethodPost,
		Path:          "/api/v1/tags",
		Summary:       "Create a tag",
		Tags:          []string{"Tags"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *tagCreateInput) (*struct{ Body tagBody }, error) {
		t, err := svc.Create(ctx, input.Body.Name)
		if err != nil {
			return nil, huma.NewError(http.StatusConflict, err.Error())
		}
		return &struct{ Body tagBody }{Body: tagBody{ID: t.ID, Name: t.Name}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-tag",
		Method:      http.MethodPut,
		Path:        "/api/v1/tags/{id}",
		Summary:     "Rename a tag",
		Tags:        []string{"Tags"},
	}, func(ctx context.Context, input *tagUpdateInput) (*struct{ Body tagBody }, error) {
		t, err := svc.Update(ctx, input.ID, input.Body.Name)
		if err != nil {
			return nil, huma.NewError(http.StatusConflict, err.Error())
		}
		return &struct{ Body tagBody }{Body: tagBody{ID: t.ID, Name: t.Name}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-tag",
		Method:        http.MethodDelete,
		Path:          "/api/v1/tags/{id}",
		Summary:       "Delete a tag",
		Tags:          []string{"Tags"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *tagDeleteInput) (*struct{}, error) {
		if err := svc.Delete(ctx, input.ID); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to delete tag", err)
		}
		return nil, nil
	})
}
