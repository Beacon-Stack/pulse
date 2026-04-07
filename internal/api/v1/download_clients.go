package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pulse/internal/core/downloadclient"
	dbsqlite "github.com/beacon-stack/pulse/internal/db/generated/sqlite"
)

type dlClientBody struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Protocol  string `json:"protocol"`
	Enabled   bool   `json:"enabled"`
	Priority  int    `json:"priority"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	UseSSL    bool   `json:"use_ssl"`
	Username  string `json:"username"`
	Category  string `json:"category"`
	Directory string `json:"directory"`
	Settings  string `json:"settings"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type dlClientCreateInput struct {
	Body struct {
		Name      string `json:"name" required:"true" minLength:"1" maxLength:"200"`
		Kind      string `json:"kind" required:"true"`
		Protocol  string `json:"protocol,omitempty"`
		Enabled   *bool  `json:"enabled,omitempty"`
		Priority  *int   `json:"priority,omitempty"`
		Host      string `json:"host" required:"true"`
		Port      int    `json:"port" required:"true"`
		UseSSL    bool   `json:"use_ssl,omitempty"`
		Username  string `json:"username,omitempty"`
		Password  string `json:"password,omitempty"`
		Category  string `json:"category,omitempty"`
		Directory string `json:"directory,omitempty"`
		Settings  string `json:"settings,omitempty"`
	}
}

type dlClientUpdateInput struct {
	ID   string `path:"id"`
	Body struct {
		Name      string `json:"name" required:"true" minLength:"1" maxLength:"200"`
		Kind      string `json:"kind" required:"true"`
		Protocol  string `json:"protocol,omitempty"`
		Enabled   *bool  `json:"enabled,omitempty"`
		Priority  *int   `json:"priority,omitempty"`
		Host      string `json:"host" required:"true"`
		Port      int    `json:"port" required:"true"`
		UseSSL    bool   `json:"use_ssl,omitempty"`
		Username  string `json:"username,omitempty"`
		Password  string `json:"password,omitempty"`
		Category  string `json:"category,omitempty"`
		Directory string `json:"directory,omitempty"`
		Settings  string `json:"settings,omitempty"`
	}
}

type dlClientIDInput struct {
	ID string `path:"id"`
}

type dlClientTestInput struct {
	Body struct {
		Kind   string `json:"kind" required:"true"`
		Host   string `json:"host" required:"true"`
		Port   int    `json:"port" required:"true"`
		UseSSL bool   `json:"use_ssl,omitempty"`
	}
}

func toDLClientBody(row interface{ GetID() string }) dlClientBody {
	// This is handled inline since the generated type doesn't have methods
	return dlClientBody{}
}

// RegisterDownloadClientRoutes registers download client management endpoints.
func RegisterDownloadClientRoutes(api huma.API, svc *downloadclient.Service) {
	// POST /api/v1/download-clients — create
	huma.Register(api, huma.Operation{
		OperationID:   "create-download-client",
		Method:        http.MethodPost,
		Path:          "/api/v1/download-clients",
		Summary:       "Add a download client",
		Tags:          []string{"Download Clients"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *dlClientCreateInput) (*struct{ Body dlClientBody }, error) {
		enabled := true
		if input.Body.Enabled != nil {
			enabled = *input.Body.Enabled
		}
		priority := 1
		if input.Body.Priority != nil {
			priority = *input.Body.Priority
		}

		row, err := svc.Create(ctx, downloadclient.Input{
			Name:      input.Body.Name,
			Kind:      input.Body.Kind,
			Protocol:  input.Body.Protocol,
			Enabled:   enabled,
			Priority:  priority,
			Host:      input.Body.Host,
			Port:      input.Body.Port,
			UseSSL:    input.Body.UseSSL,
			Username:  input.Body.Username,
			Password:  input.Body.Password,
			Category:  input.Body.Category,
			Directory: input.Body.Directory,
			Settings:  input.Body.Settings,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to create download client", err)
		}
		return &struct{ Body dlClientBody }{Body: rowToBody(row)}, nil
	})

	// GET /api/v1/download-clients — list all
	huma.Register(api, huma.Operation{
		OperationID: "list-download-clients",
		Method:      http.MethodGet,
		Path:        "/api/v1/download-clients",
		Summary:     "List all download clients",
		Tags:        []string{"Download Clients"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []dlClientBody }, error) {
		rows, err := svc.List(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list download clients", err)
		}
		items := make([]dlClientBody, len(rows))
		for i := range rows {
			items[i] = rowToBody(&rows[i])
		}
		return &struct{ Body []dlClientBody }{Body: items}, nil
	})

	// GET /api/v1/download-clients/{id} — get one
	huma.Register(api, huma.Operation{
		OperationID: "get-download-client",
		Method:      http.MethodGet,
		Path:        "/api/v1/download-clients/{id}",
		Summary:     "Get a download client by ID",
		Tags:        []string{"Download Clients"},
	}, func(ctx context.Context, input *dlClientIDInput) (*struct{ Body dlClientBody }, error) {
		row, err := svc.Get(ctx, input.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusNotFound, "download client not found", err)
		}
		return &struct{ Body dlClientBody }{Body: rowToBody(row)}, nil
	})

	// PUT /api/v1/download-clients/{id} — update
	huma.Register(api, huma.Operation{
		OperationID: "update-download-client",
		Method:      http.MethodPut,
		Path:        "/api/v1/download-clients/{id}",
		Summary:     "Update a download client",
		Tags:        []string{"Download Clients"},
	}, func(ctx context.Context, input *dlClientUpdateInput) (*struct{ Body dlClientBody }, error) {
		enabled := true
		if input.Body.Enabled != nil {
			enabled = *input.Body.Enabled
		}
		priority := 1
		if input.Body.Priority != nil {
			priority = *input.Body.Priority
		}

		row, err := svc.Update(ctx, input.ID, downloadclient.Input{
			Name:      input.Body.Name,
			Kind:      input.Body.Kind,
			Protocol:  input.Body.Protocol,
			Enabled:   enabled,
			Priority:  priority,
			Host:      input.Body.Host,
			Port:      input.Body.Port,
			UseSSL:    input.Body.UseSSL,
			Username:  input.Body.Username,
			Password:  input.Body.Password,
			Category:  input.Body.Category,
			Directory: input.Body.Directory,
			Settings:  input.Body.Settings,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to update download client", err)
		}
		return &struct{ Body dlClientBody }{Body: rowToBody(row)}, nil
	})

	// DELETE /api/v1/download-clients/{id} — delete
	huma.Register(api, huma.Operation{
		OperationID:   "delete-download-client",
		Method:        http.MethodDelete,
		Path:          "/api/v1/download-clients/{id}",
		Summary:       "Delete a download client",
		Tags:          []string{"Download Clients"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *dlClientIDInput) (*struct{}, error) {
		if err := svc.Delete(ctx, input.ID); err != nil {
			return nil, huma.NewError(http.StatusNotFound, "download client not found", err)
		}
		return nil, nil
	})

	// POST /api/v1/download-clients/test — test connectivity
	huma.Register(api, huma.Operation{
		OperationID: "test-download-client",
		Method:      http.MethodPost,
		Path:        "/api/v1/download-clients/test",
		Summary:     "Test download client connectivity",
		Tags:        []string{"Download Clients"},
	}, func(ctx context.Context, input *dlClientTestInput) (*struct{ Body downloadclient.DLClientTestResult }, error) {
		result := svc.Test(ctx, input.Body.Kind, input.Body.Host, input.Body.Port, input.Body.UseSSL)
		return &struct{ Body downloadclient.DLClientTestResult }{Body: result}, nil
	})
}

func rowToBody(row *dbsqlite.DownloadClient) dlClientBody {
	return dlClientBody{
		ID:        row.ID,
		Name:      row.Name,
		Kind:      row.Kind,
		Protocol:  row.Protocol,
		Enabled:   row.Enabled == 1,
		Priority:  int(row.Priority),
		Host:      row.Host,
		Port:      int(row.Port),
		UseSSL:    row.UseSsl == 1,
		Username:  row.Username,
		Category:  row.Category,
		Directory: row.Directory,
		Settings:  row.Settings,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
