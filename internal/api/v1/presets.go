package v1

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	dbsqlite "github.com/arrsenal/configurarr/internal/db/generated/sqlite"
)

type presetBody struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Filters   string `json:"filters"` // raw JSON
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type presetSaveInput struct {
	Body struct {
		Name    string `json:"name" required:"true" minLength:"1" maxLength:"100"`
		Filters string `json:"filters" required:"true"`
	}
}

type presetDeleteInput struct {
	ID string `path:"id"`
}

func toPresetBody(row dbsqlite.FilterPreset) presetBody {
	return presetBody{
		ID:        row.ID,
		Name:      row.Name,
		Filters:   row.Filters,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// RegisterPresetRoutes registers filter preset CRUD endpoints.
func RegisterPresetRoutes(api huma.API, q dbsqlite.Querier) {
	// GET /api/v1/presets — list all
	huma.Register(api, huma.Operation{
		OperationID: "list-presets",
		Method:      http.MethodGet,
		Path:        "/api/v1/presets",
		Summary:     "List saved filter presets",
		Tags:        []string{"Presets"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []presetBody }, error) {
		rows, err := q.ListFilterPresets(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list presets", err)
		}
		items := make([]presetBody, len(rows))
		for i, r := range rows {
			items[i] = toPresetBody(r)
		}
		return &struct{ Body []presetBody }{Body: items}, nil
	})

	// PUT /api/v1/presets — create or update (upsert by name)
	huma.Register(api, huma.Operation{
		OperationID: "save-preset",
		Method:      http.MethodPut,
		Path:        "/api/v1/presets",
		Summary:     "Save a filter preset (upsert by name)",
		Tags:        []string{"Presets"},
	}, func(ctx context.Context, input *presetSaveInput) (*struct{ Body presetBody }, error) {
		now := time.Now().UTC().Format(time.RFC3339)
		row, err := q.UpsertFilterPreset(ctx, dbsqlite.UpsertFilterPresetParams{
			ID:        uuid.New().String(),
			Name:      input.Body.Name,
			Filters:   input.Body.Filters,
			CreatedAt: now,
			UpdatedAt: now,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to save preset", err)
		}
		return &struct{ Body presetBody }{Body: toPresetBody(row)}, nil
	})

	// DELETE /api/v1/presets/{id} — delete
	huma.Register(api, huma.Operation{
		OperationID:   "delete-preset",
		Method:        http.MethodDelete,
		Path:          "/api/v1/presets/{id}",
		Summary:       "Delete a filter preset",
		Tags:          []string{"Presets"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *presetDeleteInput) (*struct{}, error) {
		if err := q.DeleteFilterPreset(ctx, input.ID); err != nil {
			return nil, huma.NewError(http.StatusNotFound, "preset not found", err)
		}
		return nil, nil
	})
}
