package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/beacon-media/pulse/internal/api/middleware"
	v1 "github.com/beacon-media/pulse/internal/api/v1"
	"github.com/beacon-media/pulse/internal/api/ws"
	appconfig "github.com/beacon-media/pulse/internal/config"
	cfgstore "github.com/beacon-media/pulse/internal/core/config"
	"github.com/beacon-media/pulse/internal/core/downloadclient"
	"github.com/beacon-media/pulse/internal/core/indexer"
	"github.com/beacon-media/pulse/internal/core/registry"
	"github.com/beacon-media/pulse/internal/core/tag"
	dbsqlite "github.com/beacon-media/pulse/internal/db/generated/sqlite"
	"github.com/beacon-media/pulse/internal/scraper"
	"github.com/beacon-media/pulse/web"
)

// RouterConfig holds everything the router needs to function.
type RouterConfig struct {
	Auth            appconfig.Secret
	Logger          *slog.Logger
	StartTime       time.Time
	RegistryService *registry.Service
	ConfigStore     *cfgstore.Store
	IndexerManager  *indexer.Manager
	TagService      *tag.Service
	WSHub           *ws.Hub
	DownloadClientService *downloadclient.Service
	ScraperEngine        *scraper.Engine
	Queries              dbsqlite.Querier
	ExternalURL          string // e.g., "http://pulse:9696" — used for Torznab proxy URL rewriting
}

// NewRouter builds and returns the application HTTP handler.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.MaxRequestBodySize(1 << 20)) // 1 MiB
	r.Use(middleware.RequestLogger(cfg.Logger))
	r.Use(middleware.Recovery(cfg.Logger))

	// WebSocket — auth handled inside the hub.
	if cfg.WSHub != nil {
		r.Get("/api/v1/ws", cfg.WSHub.ServeHTTP)
	}

	// Torznab proxy — registered directly on chi (XML responses, not JSON).
	// Auth is via the apikey query parameter (standard Torznab auth).
	if cfg.ScraperEngine != nil && cfg.Queries != nil {
		torznabHandler := v1.NewTorznabHandler(cfg.ScraperEngine, cfg.Queries, cfg.Logger)
		v1.RegisterTorznabRoutes(r, torznabHandler)
	}

	// Unauthenticated health check for container probes.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	humaConfig := huma.DefaultConfig("Pulse API", "0.1.0")
	humaConfig.DocsPath = "/api/docs"
	humaConfig.OpenAPIPath = "/api/openapi"
	humaConfig.SchemasPath = "/api/schemas"
	humaConfig.Info.Description = "Pulse — centralized control plane for the Arr ecosystem. " +
		"Manages service registration, discovery, shared configuration, and indexer assignment."

	humaAPI := humachi.New(r, humaConfig)

	// Security scheme for docs UI.
	oapi := humaAPI.OpenAPI()
	if oapi.Components == nil {
		oapi.Components = &huma.Components{}
	}
	if oapi.Components.SecuritySchemes == nil {
		oapi.Components.SecuritySchemes = map[string]*huma.SecurityScheme{}
	}
	oapi.Components.SecuritySchemes["ApiKeyAuth"] = &huma.SecurityScheme{
		Type: "apiKey",
		In:   "header",
		Name: "X-Api-Key",
	}
	oapi.Security = []map[string][]string{{"ApiKeyAuth": {}}}

	// Auth middleware.
	apiKeyBytes := []byte(cfg.Auth.Value())
	humaAPI.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		if ctx.Header("Sec-Fetch-Site") == "same-origin" {
			next(ctx)
			return
		}
		if len(apiKeyBytes) > 0 && subtle.ConstantTimeCompare([]byte(ctx.Header("X-Api-Key")), apiKeyBytes) == 1 {
			next(ctx)
			return
		}
		_ = huma.WriteErr(humaAPI, ctx, http.StatusUnauthorized, "A valid X-Api-Key header is required.")
	})

	// Register route groups.
	v1.RegisterSystemRoutes(humaAPI, cfg.StartTime)

	if cfg.RegistryService != nil {
		v1.RegisterServiceRoutes(humaAPI, cfg.RegistryService)
	}

	if cfg.ConfigStore != nil {
		v1.RegisterConfigRoutes(humaAPI, cfg.ConfigStore)
	}

	if cfg.IndexerManager != nil {
		v1.RegisterIndexerRoutes(humaAPI, cfg.IndexerManager, cfg.ExternalURL)
	}

	if cfg.TagService != nil {
		v1.RegisterTagRoutes(humaAPI, cfg.TagService)
	}

	if cfg.Queries != nil {
		v1.RegisterPresetRoutes(humaAPI, cfg.Queries)
	}

	if cfg.DownloadClientService != nil {
		v1.RegisterDownloadClientRoutes(humaAPI, cfg.DownloadClientService)
	}

	v1.RegisterCatalogRoutes(humaAPI)

	// Serve the embedded React SPA. Must come after all API routes
	// so /api/* and /health take precedence.
	r.Handle("/*", web.ServeStatic())

	return r
}
