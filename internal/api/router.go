package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/beacon-stack/pulse/pkg/auth"
	"github.com/beacon-stack/pulse/pkg/auth/authapi"

	"github.com/beacon-stack/pulse/internal/version"
	"github.com/beacon-stack/pulse/pkg/notify"
	"github.com/beacon-stack/pulse/pkg/notify/notifyapi"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/beacon-stack/pulse/internal/api/middleware"
	v1 "github.com/beacon-stack/pulse/internal/api/v1"
	"github.com/beacon-stack/pulse/internal/api/ws"
	appconfig "github.com/beacon-stack/pulse/internal/config"
	cfgstore "github.com/beacon-stack/pulse/internal/core/config"
	"github.com/beacon-stack/pulse/internal/core/dashboard"
	"github.com/beacon-stack/pulse/internal/core/downloadclient"
	"github.com/beacon-stack/pulse/internal/core/gluetun"
	"github.com/beacon-stack/pulse/internal/core/indexer"
	"github.com/beacon-stack/pulse/internal/core/indexerprobe"
	"github.com/beacon-stack/pulse/internal/core/qualityprofile"
	"github.com/beacon-stack/pulse/internal/core/registry"
	"github.com/beacon-stack/pulse/internal/core/sharedsettings"
	"github.com/beacon-stack/pulse/internal/core/tag"
	db "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/scraper"
	"github.com/beacon-stack/pulse/pkg/log"
	"github.com/beacon-stack/pulse/web"
)

// RouterConfig holds everything the router needs to function.
type RouterConfig struct {
	// Gate decides who may use the app. When nil, one is built from Auth
	// with default settings: no login, local network only.
	Gate                  *auth.Auth
	Auth                  appconfig.Secret
	Logger                *slog.Logger
	StartTime             time.Time
	DBPath                string // shown on the System page
	RegistryService       *registry.Service
	IndexerManager        *indexer.Manager
	TagService            *tag.Service
	WSHub                 *ws.Hub
	DownloadClientService *downloadclient.Service
	QualityProfileService *qualityprofile.Service
	SharedSettingsService *sharedsettings.Service
	ScraperEngine         *scraper.Engine
	CatalogProber         *indexerprobe.Prober // nil leaves the catalog tester API out
	ConfigStore           *cfgstore.Store      // nil leaves the UI settings API out
	Queries               db.Querier
	DashboardAggregator   *dashboard.Aggregator
	VPNRotator            *gluetun.Rotator // nil when gluetun is not configured
	ExternalURL           string           // e.g., "http://pulse:9696" — used for Torznab proxy URL rewriting
	// LogSystem is the handle returned by log.New. When non-nil, the
	// router exposes /api/v1/system/logs and /api/v1/system/log-level
	// for the in-app viewer. DockerLogs (optional) adds the
	// /api/v1/system/logs/docker passthrough for full history.
	LogSystem  *log.System
	DockerLogs *log.DockerLogsReader
	// Notifications: Pulse's own channels, which broadcast events the
	// apps send it. Nil leaves the notifications API out.
	NotifyStore  *notify.Store
	NotifyOutbox *notify.Outbox
}

// NewRouter builds and returns the application HTTP handler.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.MaxRequestBodySize(1 << 20)) // 1 MiB
	r.Use(middleware.RequestLogger(cfg.Logger))
	r.Use(middleware.Recovery(cfg.Logger))

	// Who may use the app (pkg/auth): by default the local network without
	// a login. Login, logout and status are the only open /api routes.
	gate := cfg.Gate
	if gate == nil {
		gate = auth.New(auth.Options{App: "Pulse", APIKey: cfg.Auth.Value()}, auth.Settings{})
	}
	r.Get("/api/v1/auth/status", gate.StatusHandler())
	r.Post("/api/v1/auth/login", gate.LoginHandler())
	r.Post("/api/v1/auth/logout", gate.LogoutHandler())

	// Everything registered on protected goes through the gate. It is
	// attached per route, not by path prefix, so nothing routes around it.
	protected := r.With(gate.Middleware)

	// WebSocket. On chi directly so the upgrade bypasses huma.
	if cfg.WSHub != nil {
		protected.Get("/api/v1/ws", cfg.WSHub.ServeHTTP)
	}

	// Torznab proxy, on chi directly (XML responses, not JSON). The apps
	// reach it over the container network, which counts as local.
	if cfg.ScraperEngine != nil && cfg.Queries != nil {
		torznabHandler := v1.NewTorznabHandler(cfg.ScraperEngine, cfg.Queries, cfg.IndexerManager, cfg.ExternalURL, cfg.Logger)
		v1.RegisterTorznabRoutes(protected, torznabHandler)
	}

	// Unauthenticated health check for container probes.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	humaConfig := huma.DefaultConfig("Pulse API", version.Version)
	humaConfig.DocsPath = "/api/docs"
	humaConfig.OpenAPIPath = "/api/openapi"
	humaConfig.SchemasPath = "/api/schemas"
	humaConfig.Info.Description = "Pulse — centralized control plane for the Arr ecosystem. " +
		"Manages service registration, discovery, shared configuration, and indexer assignment."

	humaAPI := humachi.New(protected, humaConfig)

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

	authapi.Register(humaAPI, gate)

	// Register route groups.
	v1.RegisterSystemRoutes(humaAPI, cfg.StartTime, cfg.DBPath)
	if cfg.NotifyStore != nil && cfg.NotifyOutbox != nil {
		notifyapi.Register(humaAPI, notifyapi.Options{App: "Pulse", Store: cfg.NotifyStore, Outbox: cfg.NotifyOutbox, Hub: true})
	}
	v1.RegisterUpdateRoutes(humaAPI)
	v1.RegisterRuntimeRoutes(humaAPI, cfg.StartTime)
	v1.RegisterEnvRoutes(humaAPI)
	if cfg.LogSystem != nil {
		log.RegisterRoutesWithDocker(humaAPI, cfg.LogSystem, cfg.DockerLogs)
	}

	if cfg.RegistryService != nil {
		v1.RegisterServiceRoutes(humaAPI, cfg.RegistryService)
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
		v1.RegisterDownloadClientRoutes(humaAPI, cfg.DownloadClientService, serviceKeyLookup(cfg.RegistryService))
	}

	if cfg.QualityProfileService != nil {
		v1.RegisterQualityProfileRoutes(humaAPI, cfg.QualityProfileService)
	}

	if cfg.SharedSettingsService != nil {
		v1.RegisterSharedSettingsRoutes(humaAPI, cfg.SharedSettingsService)
	}

	v1.RegisterCatalogRoutes(humaAPI)
	if cfg.CatalogProber != nil {
		v1.RegisterCatalogHealthRoutes(humaAPI, cfg.CatalogProber)
	}
	if cfg.ConfigStore != nil {
		v1.RegisterUISettingsRoutes(humaAPI, cfg.ConfigStore)
	}

	if cfg.DashboardAggregator != nil {
		v1.RegisterDashboardRoutes(humaAPI, cfg.DashboardAggregator)
	}

	if cfg.VPNRotator != nil {
		var troubles func() []scraper.SiteTrouble
		if cfg.ScraperEngine != nil {
			troubles = cfg.ScraperEngine.ChallengeTroubles
		}
		v1.RegisterVPNRoutes(humaAPI, cfg.VPNRotator, troubles)
	}

	// Serve the embedded React SPA. Must come after all API routes
	// so /api/* and /health take precedence.
	r.Handle("/*", web.ServeStatic())

	return r
}

// serviceKeyLookup finds the API key of the registered service listening
// at host:port, by its registered API address.
func serviceKeyLookup(reg *registry.Service) v1.ServiceKeyFunc {
	if reg == nil {
		return nil
	}
	return func(ctx context.Context, host string, port int) string {
		services, err := reg.List(ctx)
		if err != nil {
			return ""
		}
		want := net.JoinHostPort(strings.ToLower(host), strconv.Itoa(port))
		for _, svc := range services {
			u, err := url.Parse(svc.ApiUrl)
			if err != nil || u.Host == "" {
				continue
			}
			p := u.Port()
			if p == "" {
				p = "80"
				if u.Scheme == "https" {
					p = "443"
				}
			}
			if net.JoinHostPort(strings.ToLower(u.Hostname()), p) == want {
				return svc.ApiKey
			}
		}
		return ""
	}
}
