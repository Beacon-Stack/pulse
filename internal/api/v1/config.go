package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	cfgstore "github.com/beacon-stack/pulse/internal/core/config"
)

// ── Request / Response types ───────────────���─────────────────────────────────

type configEntryBody struct {
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	UpdatedAt string `json:"updated_at"`
}

type configSetInput struct {
	Body struct {
		Namespace string `json:"namespace" required:"true" minLength:"1" maxLength:"100"`
		Key       string `json:"key" required:"true" minLength:"1" maxLength:"200"`
		Value     string `json:"value" required:"true"`
	}
}

type configNamespaceInput struct {
	Namespace string `path:"namespace"`
}

type configKeyInput struct {
	Namespace string `path:"namespace"`
	Key       string `path:"key"`
}

type configSubscribeInput struct {
	Body struct {
		ServiceID string `json:"service_id" required:"true"`
		Namespace string `json:"namespace" required:"true"`
	}
}

type configUnsubscribeInput struct {
	Body struct {
		ServiceID string `json:"service_id" required:"true"`
		Namespace string `json:"namespace" required:"true"`
	}
}

type configSubscriptionsInput struct {
	ServiceID string `path:"service_id"`
}

func toConfigEntryBody(e cfgstore.Entry) configEntryBody {
	return configEntryBody{
		Namespace: e.Namespace,
		Key:       e.Key,
		Value:     e.Value,
		UpdatedAt: e.UpdatedAt,
	}
}

// RegisterConfigRoutes registers shared configuration endpoints.
func RegisterConfigRoutes(api huma.API, store *cfgstore.Store) {
	// PUT /api/v1/config — set a config entry
	huma.Register(api, huma.Operation{
		OperationID: "set-config",
		Method:      http.MethodPut,
		Path:        "/api/v1/config",
		Summary:     "Set a shared config entry (upsert)",
		Tags:        []string{"Config"},
	}, func(ctx context.Context, input *configSetInput) (*struct{ Body configEntryBody }, error) {
		entry, err := store.Set(ctx, input.Body.Namespace, input.Body.Key, input.Body.Value)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to set config", err)
		}
		return &struct{ Body configEntryBody }{Body: toConfigEntryBody(*entry)}, nil
	})

	// GET /api/v1/config — list all config entries
	huma.Register(api, huma.Operation{
		OperationID: "list-all-config",
		Method:      http.MethodGet,
		Path:        "/api/v1/config",
		Summary:     "List all shared config entries",
		Tags:        []string{"Config"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []configEntryBody }, error) {
		entries, err := store.ListAll(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list config", err)
		}
		items := make([]configEntryBody, len(entries))
		for i, e := range entries {
			items[i] = toConfigEntryBody(e)
		}
		return &struct{ Body []configEntryBody }{Body: items}, nil
	})

	// GET /api/v1/config/namespaces — list all namespaces
	huma.Register(api, huma.Operation{
		OperationID: "list-config-namespaces",
		Method:      http.MethodGet,
		Path:        "/api/v1/config/namespaces",
		Summary:     "List all config namespaces",
		Tags:        []string{"Config"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []string }, error) {
		ns, err := store.ListNamespaces(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list namespaces", err)
		}
		return &struct{ Body []string }{Body: ns}, nil
	})

	// GET /api/v1/config/{namespace} — list entries in a namespace
	huma.Register(api, huma.Operation{
		OperationID: "list-config-by-namespace",
		Method:      http.MethodGet,
		Path:        "/api/v1/config/{namespace}",
		Summary:     "List config entries in a namespace",
		Tags:        []string{"Config"},
	}, func(ctx context.Context, input *configNamespaceInput) (*struct{ Body []configEntryBody }, error) {
		entries, err := store.ListNamespace(ctx, input.Namespace)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list config", err)
		}
		items := make([]configEntryBody, len(entries))
		for i, e := range entries {
			items[i] = toConfigEntryBody(e)
		}
		return &struct{ Body []configEntryBody }{Body: items}, nil
	})

	// GET /api/v1/config/{namespace}/{key} — get a single entry
	huma.Register(api, huma.Operation{
		OperationID: "get-config-entry",
		Method:      http.MethodGet,
		Path:        "/api/v1/config/{namespace}/{key}",
		Summary:     "Get a single config entry",
		Tags:        []string{"Config"},
	}, func(ctx context.Context, input *configKeyInput) (*struct{ Body configEntryBody }, error) {
		entry, err := store.Get(ctx, input.Namespace, input.Key)
		if err != nil {
			return nil, huma.NewError(http.StatusNotFound, "config entry not found", err)
		}
		return &struct{ Body configEntryBody }{Body: toConfigEntryBody(*entry)}, nil
	})

	// DELETE /api/v1/config/{namespace}/{key} — delete a single entry
	huma.Register(api, huma.Operation{
		OperationID:   "delete-config-entry",
		Method:        http.MethodDelete,
		Path:          "/api/v1/config/{namespace}/{key}",
		Summary:       "Delete a config entry",
		Tags:          []string{"Config"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *configKeyInput) (*struct{}, error) {
		if err := store.Delete(ctx, input.Namespace, input.Key); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to delete", err)
		}
		return nil, nil
	})

	// POST /api/v1/config/subscribe — subscribe a service to a namespace
	huma.Register(api, huma.Operation{
		OperationID:   "subscribe-config",
		Method:        http.MethodPost,
		Path:          "/api/v1/config/subscribe",
		Summary:       "Subscribe a service to config namespace updates",
		Tags:          []string{"Config"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *configSubscribeInput) (*struct{}, error) {
		if err := store.Subscribe(ctx, input.Body.ServiceID, input.Body.Namespace); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "subscribe failed", err)
		}
		return nil, nil
	})

	// POST /api/v1/config/unsubscribe — unsubscribe a service
	huma.Register(api, huma.Operation{
		OperationID:   "unsubscribe-config",
		Method:        http.MethodPost,
		Path:          "/api/v1/config/unsubscribe",
		Summary:       "Unsubscribe a service from config namespace updates",
		Tags:          []string{"Config"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *configUnsubscribeInput) (*struct{}, error) {
		if err := store.Unsubscribe(ctx, input.Body.ServiceID, input.Body.Namespace); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "unsubscribe failed", err)
		}
		return nil, nil
	})

	// GET /api/v1/config/subscriptions/{service_id} — list subscriptions
	huma.Register(api, huma.Operation{
		OperationID: "list-config-subscriptions",
		Method:      http.MethodGet,
		Path:        "/api/v1/config/subscriptions/{service_id}",
		Summary:     "List config namespaces a service is subscribed to",
		Tags:        []string{"Config"},
	}, func(ctx context.Context, input *configSubscriptionsInput) (*struct{ Body []string }, error) {
		ns, err := store.ListSubscriptions(ctx, input.ServiceID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list subscriptions", err)
		}
		return &struct{ Body []string }{Body: ns}, nil
	})
}
