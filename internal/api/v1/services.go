package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pulse/internal/core/registry"
)

// ── Request / Response types ─────────────────────────────────────────────────

type serviceBody struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	APIURL       string   `json:"api_url"`
	HealthURL    string   `json:"health_url"`
	Version      string   `json:"version"`
	Status       string   `json:"status"`
	LastSeen     string   `json:"last_seen"`
	Registered   string   `json:"registered"`
	Capabilities []string `json:"capabilities"`
	Metadata     string   `json:"metadata"`
}

type registerInput struct {
	Body struct {
		Name         string   `json:"name" required:"true" minLength:"1" maxLength:"200"`
		Type         string   `json:"type" required:"true" minLength:"1" maxLength:"50"`
		APIURL       string   `json:"api_url" required:"true" minLength:"1"`
		APIKey       string   `json:"api_key,omitempty"`
		HealthURL    string   `json:"health_url,omitempty"`
		Version      string   `json:"version,omitempty"`
		Capabilities []string `json:"capabilities,omitempty"`
		Metadata     string   `json:"metadata,omitempty"`
	}
}

type serviceIDInput struct {
	ID string `path:"id"`
}

type discoverInput struct {
	Type       string `query:"type"`
	Capability string `query:"capability"`
}

func toServiceBody(s registry.ServiceInfo) serviceBody {
	caps := s.Capabilities
	if caps == nil {
		caps = []string{}
	}
	return serviceBody{
		ID:           s.ID,
		Name:         s.Name,
		Type:         s.Type,
		APIURL:       s.ApiUrl,
		HealthURL:    s.HealthUrl,
		Version:      s.Version,
		Status:       s.Status,
		LastSeen:     s.LastSeen,
		Registered:   s.Registered,
		Capabilities: caps,
		Metadata:     s.Metadata,
	}
}

// RegisterServiceRoutes registers service registry and discovery endpoints.
func RegisterServiceRoutes(api huma.API, svc *registry.Service) {
	// POST /api/v1/services/register
	huma.Register(api, huma.Operation{
		OperationID:   "register-service",
		Method:        http.MethodPost,
		Path:          "/api/v1/services/register",
		Summary:       "Register or re-register a service",
		Tags:          []string{"Services"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *registerInput) (*struct{ Body serviceBody }, error) {
		info, err := svc.Register(ctx, registry.ServiceInput{
			Name:         input.Body.Name,
			Type:         input.Body.Type,
			APIURL:       input.Body.APIURL,
			APIKey:       input.Body.APIKey,
			HealthURL:    input.Body.HealthURL,
			Version:      input.Body.Version,
			Capabilities: input.Body.Capabilities,
			Metadata:     input.Body.Metadata,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "registration failed", err)
		}
		return &struct{ Body serviceBody }{Body: toServiceBody(*info)}, nil
	})

	// GET /api/v1/services
	huma.Register(api, huma.Operation{
		OperationID: "list-services",
		Method:      http.MethodGet,
		Path:        "/api/v1/services",
		Summary:     "List all registered services",
		Tags:        []string{"Services"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []serviceBody }, error) {
		list, err := svc.List(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list services", err)
		}
		items := make([]serviceBody, len(list))
		for i, s := range list {
			items[i] = toServiceBody(s)
		}
		return &struct{ Body []serviceBody }{Body: items}, nil
	})

	// GET /api/v1/services/discover?type=...&capability=...
	huma.Register(api, huma.Operation{
		OperationID: "discover-services",
		Method:      http.MethodGet,
		Path:        "/api/v1/services/discover",
		Summary:     "Discover services by type or capability",
		Tags:        []string{"Services"},
	}, func(ctx context.Context, input *discoverInput) (*struct{ Body []serviceBody }, error) {
		var list []registry.ServiceInfo
		var err error

		switch {
		case input.Capability != "":
			list, err = svc.ListByCapability(ctx, input.Capability)
		case input.Type != "":
			list, err = svc.ListByType(ctx, input.Type)
		default:
			list, err = svc.List(ctx)
		}
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "discovery failed", err)
		}
		items := make([]serviceBody, len(list))
		for i, s := range list {
			items[i] = toServiceBody(s)
		}
		return &struct{ Body []serviceBody }{Body: items}, nil
	})

	// GET /api/v1/services/{id}
	huma.Register(api, huma.Operation{
		OperationID: "get-service",
		Method:      http.MethodGet,
		Path:        "/api/v1/services/{id}",
		Summary:     "Get a service by ID",
		Tags:        []string{"Services"},
	}, func(ctx context.Context, input *serviceIDInput) (*struct{ Body serviceBody }, error) {
		info, err := svc.Get(ctx, input.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusNotFound, "service not found", err)
		}
		return &struct{ Body serviceBody }{Body: toServiceBody(*info)}, nil
	})

	// PUT /api/v1/services/{id}/heartbeat
	huma.Register(api, huma.Operation{
		OperationID:   "heartbeat-service",
		Method:        http.MethodPut,
		Path:          "/api/v1/services/{id}/heartbeat",
		Summary:       "Send heartbeat for a service",
		Tags:          []string{"Services"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *serviceIDInput) (*struct{}, error) {
		if err := svc.Heartbeat(ctx, input.ID); err != nil {
			return nil, huma.NewError(http.StatusNotFound, "service not found", err)
		}
		return nil, nil
	})

	// DELETE /api/v1/services/{id}
	huma.Register(api, huma.Operation{
		OperationID:   "deregister-service",
		Method:        http.MethodDelete,
		Path:          "/api/v1/services/{id}",
		Summary:       "Deregister a service",
		Tags:          []string{"Services"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *serviceIDInput) (*struct{}, error) {
		if err := svc.Deregister(ctx, input.ID); err != nil {
			return nil, huma.NewError(http.StatusNotFound, "service not found", err)
		}
		return nil, nil
	})
}
