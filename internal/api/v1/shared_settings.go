package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pulse/internal/core/sharedsettings"
)

type sharedSettingsBody struct {
	ColonReplacement    string `json:"colon_replacement"`
	ImportExtraFiles    bool   `json:"import_extra_files"`
	ExtraFileExtensions string `json:"extra_file_extensions"`
	RenameFiles         bool   `json:"rename_files"`
	UpdatedAt           string `json:"updated_at"`
}

type sharedSettingsUpdateInput struct {
	Body struct {
		ColonReplacement    string `json:"colon_replacement" required:"true" enum:"delete,dash,space-dash,smart"`
		ImportExtraFiles    bool   `json:"import_extra_files"`
		ExtraFileExtensions string `json:"extra_file_extensions"`
		RenameFiles         bool   `json:"rename_files"`
	}
}

func toSharedSettingsBody(s *sharedsettings.Settings) sharedSettingsBody {
	return sharedSettingsBody{
		ColonReplacement:    s.ColonReplacement,
		ImportExtraFiles:    s.ImportExtraFiles,
		ExtraFileExtensions: s.ExtraFileExtensions,
		RenameFiles:         s.RenameFiles,
		UpdatedAt:           s.UpdatedAt,
	}
}

// RegisterSharedSettingsRoutes registers the shared media handling endpoints.
func RegisterSharedSettingsRoutes(api huma.API, svc *sharedsettings.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "get-shared-settings",
		Method:      http.MethodGet,
		Path:        "/api/v1/shared-settings",
		Summary:     "Get shared media handling settings",
		Tags:        []string{"SharedSettings"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body sharedSettingsBody }, error) {
		s, err := svc.Get(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to read shared settings", err)
		}
		return &struct{ Body sharedSettingsBody }{Body: toSharedSettingsBody(s)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-shared-settings",
		Method:      http.MethodPut,
		Path:        "/api/v1/shared-settings",
		Summary:     "Update shared media handling settings",
		Tags:        []string{"SharedSettings"},
	}, func(ctx context.Context, input *sharedSettingsUpdateInput) (*struct{ Body sharedSettingsBody }, error) {
		s, err := svc.Update(ctx, sharedsettings.Settings{
			ColonReplacement:    input.Body.ColonReplacement,
			ImportExtraFiles:    input.Body.ImportExtraFiles,
			ExtraFileExtensions: input.Body.ExtraFileExtensions,
			RenameFiles:         input.Body.RenameFiles,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to update shared settings", err)
		}
		return &struct{ Body sharedSettingsBody }{Body: toSharedSettingsBody(s)}, nil
	})
}
