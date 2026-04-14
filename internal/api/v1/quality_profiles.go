package v1

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pulse/internal/core/qualityprofile"
	db "github.com/beacon-stack/pulse/internal/db/generated"
)

// qualityProfileBody is the JSON response shape for a quality profile.
// All JSON fields (cutoff_json, qualities_json, upgrade_until_json) are
// strings that services parse themselves — Pulse does not interpret them.
type qualityProfileBody struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	CutoffJSON           string  `json:"cutoff_json"`
	QualitiesJSON        string  `json:"qualities_json"`
	UpgradeAllowed       bool    `json:"upgrade_allowed"`
	UpgradeUntilJSON     *string `json:"upgrade_until_json,omitempty"`
	MinCustomFormatScore int     `json:"min_custom_format_score"`
	UpgradeUntilCFScore  int     `json:"upgrade_until_cf_score"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

type qualityProfileInputBody struct {
	Name                 string  `json:"name" required:"true" minLength:"1" maxLength:"200"`
	CutoffJSON           string  `json:"cutoff_json" required:"true"`
	QualitiesJSON        string  `json:"qualities_json" required:"true"`
	UpgradeAllowed       bool    `json:"upgrade_allowed"`
	UpgradeUntilJSON     *string `json:"upgrade_until_json,omitempty"`
	MinCustomFormatScore int     `json:"min_custom_format_score,omitempty"`
	UpgradeUntilCFScore  int     `json:"upgrade_until_cf_score,omitempty"`
}

type qualityProfileCreateInput struct {
	Body qualityProfileInputBody
}

type qualityProfileUpdateInput struct {
	ID   string `path:"id"`
	Body qualityProfileInputBody
}

type qualityProfileIDInput struct {
	ID string `path:"id"`
}

func toQualityProfileBody(row *db.QualityProfile) qualityProfileBody {
	var upgradeUntil *string
	if row.UpgradeUntilJson.Valid {
		s := row.UpgradeUntilJson.String
		upgradeUntil = &s
	}
	return qualityProfileBody{
		ID:                   row.ID,
		Name:                 row.Name,
		CutoffJSON:           row.CutoffJson,
		QualitiesJSON:        row.QualitiesJson,
		UpgradeAllowed:       row.UpgradeAllowed,
		UpgradeUntilJSON:     upgradeUntil,
		MinCustomFormatScore: int(row.MinCustomFormatScore),
		UpgradeUntilCFScore:  int(row.UpgradeUntilCfScore),
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
}

func inputToServiceInput(body qualityProfileInputBody) qualityprofile.Input {
	return qualityprofile.Input{
		Name:                 body.Name,
		CutoffJSON:           body.CutoffJSON,
		QualitiesJSON:        body.QualitiesJSON,
		UpgradeAllowed:       body.UpgradeAllowed,
		UpgradeUntilJSON:     body.UpgradeUntilJSON,
		MinCustomFormatScore: body.MinCustomFormatScore,
		UpgradeUntilCFScore:  body.UpgradeUntilCFScore,
	}
}

// RegisterQualityProfileRoutes registers quality profile management endpoints.
func RegisterQualityProfileRoutes(api huma.API, svc *qualityprofile.Service) {
	// POST /api/v1/quality-profiles — create
	huma.Register(api, huma.Operation{
		OperationID:   "create-quality-profile",
		Method:        http.MethodPost,
		Path:          "/api/v1/quality-profiles",
		Summary:       "Create a quality profile",
		Tags:          []string{"Quality Profiles"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *qualityProfileCreateInput) (*struct{ Body qualityProfileBody }, error) {
		row, err := svc.Create(ctx, inputToServiceInput(input.Body))
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to create quality profile", err)
		}
		return &struct{ Body qualityProfileBody }{Body: toQualityProfileBody(row)}, nil
	})

	// GET /api/v1/quality-profiles — list all
	huma.Register(api, huma.Operation{
		OperationID: "list-quality-profiles",
		Method:      http.MethodGet,
		Path:        "/api/v1/quality-profiles",
		Summary:     "List all quality profiles",
		Tags:        []string{"Quality Profiles"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []qualityProfileBody }, error) {
		rows, err := svc.List(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list quality profiles", err)
		}
		items := make([]qualityProfileBody, len(rows))
		for i := range rows {
			items[i] = toQualityProfileBody(&rows[i])
		}
		return &struct{ Body []qualityProfileBody }{Body: items}, nil
	})

	// GET /api/v1/quality-profiles/{id} — get one
	huma.Register(api, huma.Operation{
		OperationID: "get-quality-profile",
		Method:      http.MethodGet,
		Path:        "/api/v1/quality-profiles/{id}",
		Summary:     "Get a quality profile by ID",
		Tags:        []string{"Quality Profiles"},
	}, func(ctx context.Context, input *qualityProfileIDInput) (*struct{ Body qualityProfileBody }, error) {
		row, err := svc.Get(ctx, input.ID)
		if err != nil {
			if errors.Is(err, qualityprofile.ErrNotFound) {
				return nil, huma.NewError(http.StatusNotFound, "quality profile not found", err)
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get quality profile", err)
		}
		return &struct{ Body qualityProfileBody }{Body: toQualityProfileBody(row)}, nil
	})

	// PUT /api/v1/quality-profiles/{id} — update
	huma.Register(api, huma.Operation{
		OperationID: "update-quality-profile",
		Method:      http.MethodPut,
		Path:        "/api/v1/quality-profiles/{id}",
		Summary:     "Update a quality profile",
		Tags:        []string{"Quality Profiles"},
	}, func(ctx context.Context, input *qualityProfileUpdateInput) (*struct{ Body qualityProfileBody }, error) {
		row, err := svc.Update(ctx, input.ID, inputToServiceInput(input.Body))
		if err != nil {
			if errors.Is(err, qualityprofile.ErrNotFound) {
				return nil, huma.NewError(http.StatusNotFound, "quality profile not found", err)
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to update quality profile", err)
		}
		return &struct{ Body qualityProfileBody }{Body: toQualityProfileBody(row)}, nil
	})

	// DELETE /api/v1/quality-profiles/{id} — delete
	huma.Register(api, huma.Operation{
		OperationID:   "delete-quality-profile",
		Method:        http.MethodDelete,
		Path:          "/api/v1/quality-profiles/{id}",
		Summary:       "Delete a quality profile",
		Tags:          []string{"Quality Profiles"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *qualityProfileIDInput) (*struct{}, error) {
		if err := svc.Delete(ctx, input.ID); err != nil {
			if errors.Is(err, qualityprofile.ErrNotFound) {
				return nil, huma.NewError(http.StatusNotFound, "quality profile not found", err)
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to delete quality profile", err)
		}
		return nil, nil
	})
}
