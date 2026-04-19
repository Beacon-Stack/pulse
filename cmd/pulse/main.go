package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/beacon-stack/pulse/internal/api"
	"github.com/beacon-stack/pulse/internal/api/ws"
	"github.com/beacon-stack/pulse/internal/config"
	cfgstore "github.com/beacon-stack/pulse/internal/core/config"
	"github.com/beacon-stack/pulse/internal/core/health"
	"github.com/beacon-stack/pulse/internal/core/indexer"
	"github.com/beacon-stack/pulse/internal/core/registry"
	"github.com/beacon-stack/pulse/internal/core/tag"
	"github.com/beacon-stack/pulse/internal/db"
	"github.com/beacon-stack/pulse/internal/core/downloadclient"
	"github.com/beacon-stack/pulse/internal/core/qualityprofile"
	"github.com/beacon-stack/pulse/internal/core/sharedsettings"
	dbgen "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/events"
	"github.com/beacon-stack/pulse/internal/scraper"
)

func main() {
	configPath := flag.String("config", "", "path to config.yaml")
	flag.Parse()

	// ── Load configuration ───────────────────────────────────────────────
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pulse: %v\n", err)
		os.Exit(1)
	}

	// ── Logger ───────────────────────────────────────────────────────────
	var logLevel slog.Level
	switch cfg.Log.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: logLevel}
	if cfg.Log.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	logger := slog.New(handler)

	logger.Info("starting pulse",
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
		"db_driver", cfg.Database.Driver,
	)

	if cfg.ConfigFile != "" {
		logger.Info("loaded config file", "path", cfg.ConfigFile)
	}

	// ── Database ─────────────────────────────────────────────────────────
	database, err := db.Open(cfg.Database)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := db.Migrate(database.SQL); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("database migrations complete")

	// ── Event bus ────────────────────────────────────────────────────────
	bus := events.New(logger)

	// ── SQLC queries ─────────────────────────────────────────────────────
	queries := dbgen.New(database.SQL)

	// ── Core services ────────────────────────────────────────────────────
	registrySvc := registry.NewService(queries, bus, logger)
	configStore := cfgstore.NewStore(queries, bus, logger)
	indexerMgr := indexer.NewManager(queries, bus, logger)

	// Resolve the API key: env/file override first, else DB, else generate.
	// Has to happen AFTER db.Migrate (config_entries table must exist) and
	// BEFORE anything wires cfg.Auth.APIKey into the HTTP stack.
	preExisting := cfg.Auth.APIKey != ""
	if generated, err := config.EnsureAPIKey(context.Background(), configStore, cfg); err != nil {
		logger.Error("failed to resolve API key", "error", err)
		os.Exit(1)
	} else if generated {
		logger.Info("generated new API key and stored it in the database")
	} else if preExisting {
		logger.Info("API key sourced from env/file override")
	} else {
		logger.Info("loaded API key from database")
	}

	// Retroactively auto-assign existing indexers whenever a service registers
	// (or re-registers). Without this hook, a service that comes online after
	// indexers were added never gets those indexers until they're manually
	// assigned via the UI.
	registrySvc.SetOnRegister(func(ctx context.Context, serviceID string) {
		indexerMgr.AutoAssigner().AssignExistingIndexersToService(ctx, serviceID)
	})

	tagSvc := tag.NewService(queries)
	dlClientSvc := downloadclient.NewService(queries, bus, logger)
	qualityProfileSvc := qualityprofile.NewService(queries, bus, logger)
	if err := qualityProfileSvc.SeedDefaults(context.Background(), configStore); err != nil {
		logger.Warn("failed to seed default quality profiles", "error", err)
	}
	sharedSettingsSvc := sharedsettings.NewService(queries, bus, logger)
	healthChecker := health.NewChecker(queries, bus, logger)

	// When a download client changes, notify all services that support its protocol.
	dlClientPusher := indexer.NewPusher(queries, logger) // reuse the indexer pusher for notifications
	dlClientSvc.SetNotifier(func(ctx context.Context, protocol string) {
		capability := "supports_" + protocol // supports_torrent or supports_usenet
		services, err := queries.ListServicesByCapability(ctx, capability)
		if err != nil {
			logger.Warn("download-client: failed to list services for push", "error", err)
			return
		}
		for _, svc := range services {
			dlClientPusher.NotifyServiceAsync(svc.ID)
		}
	})

	// When a new service registers, auto-assign existing indexers based on
	// category↔capability matching (e.g., Movies indexers → content:movies services).
	// Also auto-register download-client type services as download clients.
	registrySvc.SetOnRegister(func(ctx context.Context, serviceID string) {
		indexerMgr.AutoAssigner().AssignExistingIndexersToService(ctx, serviceID)

		// Auto-register download-client services.
		svcInfo, err := registrySvc.Get(ctx, serviceID)
		if err != nil {
			return
		}
		if svcInfo.Type != "download-client" {
			return
		}

		// Parse host and port from the service's API URL.
		host, port := parseHostPort(svcInfo.ApiUrl)
		if host == "" {
			return
		}

		// Determine the kind from the service name.
		kind := svcInfo.Name

		// Determine protocol from capabilities.
		protocol := "torrent"
		for _, cap := range svcInfo.Capabilities {
			if cap == "supports_usenet" {
				protocol = "usenet"
			}
		}

		// Check if a download client with this name already exists.
		// If it does, update it (service may have restarted on a new port
		// or regenerated its API key). If not, create it.
		existing, _ := dlClientSvc.List(ctx)
		for _, e := range existing {
			if e.Name == svcInfo.Name {
				_, updateErr := dlClientSvc.Update(ctx, e.ID, downloadclient.Input{
					Name:     svcInfo.Name,
					Kind:     kind,
					Protocol: protocol,
					Enabled:  e.Enabled,
					Priority: int(e.Priority),
					Host:     host,
					Port:     port,
					Password: svcInfo.ApiKey,
					Settings: `{"pulse":true}`,
				})
				if updateErr != nil {
					logger.Warn("pulse: failed to update download-client on re-register",
						"name", svcInfo.Name, "error", updateErr)
				} else {
					logger.Info("pulse: updated download-client service on re-register",
						"name", svcInfo.Name, "host", host, "port", port)
				}
				return
			}
		}

		_, err = dlClientSvc.Create(ctx, downloadclient.Input{
			Name:     svcInfo.Name,
			Kind:     kind,
			Protocol: protocol,
			Enabled:  true,
			Priority: 1,
			Host:     host,
			Port:     port,
			Password: svcInfo.ApiKey,
			Settings: `{"pulse":true}`,
		})
		if err != nil {
			logger.Warn("pulse: failed to auto-register download client",
				"name", svcInfo.Name, "error", err)
			return
		}
		logger.Info("pulse: auto-registered download-client service",
			"name", svcInfo.Name, "host", host, "port", port)
	})

	// ── Prowlarr-sourced indexer catalog ──────────────────────────────────
	prowlarrCatalog := indexer.NewProwlarrCatalog(logger)
	indexer.SetCatalogSource(prowlarrCatalog.Entries)

	// ── FlareSolverr (optional) ───────────────────────────────────────────
	flaresolverr := scraper.NewFlareSolverr(cfg.FlareSolverr.URL, logger)
	if flaresolverr != nil {
		logger.Info("FlareSolverr configured", "url", cfg.FlareSolverr.URL)
	}

	// ── Scraper engine ───────────────────────────────────────────────────
	scraperEngine := scraper.NewEngine(logger, flaresolverr)

	// ── WebSocket hub ────────────────────────────────────────────────────
	wsHub := ws.NewHub(logger, []byte(cfg.Auth.APIKey.Value()))
	bus.Subscribe(wsHub.HandleEvent)

	// ── HTTP router ──────────────────────────────────────────────────────
	// Compute external URL for Torznab proxy URL rewriting.
	externalURL := cfg.Server.ExternalURL
	if externalURL == "" {
		host := cfg.Server.Host
		if host == "0.0.0.0" || host == "" {
			host = "localhost"
		}
		externalURL = fmt.Sprintf("http://%s:%d", host, cfg.Server.Port)
	}

	startTime := time.Now()
	router := api.NewRouter(api.RouterConfig{
		Auth:            cfg.Auth.APIKey,
		Logger:          logger,
		StartTime:       startTime,
		RegistryService: registrySvc,
		IndexerManager:  indexerMgr,
		TagService:              tagSvc,
		DownloadClientService:   dlClientSvc,
		QualityProfileService:   qualityProfileSvc,
		SharedSettingsService:   sharedSettingsSvc,
		WSHub:           wsHub,
		ScraperEngine:   scraperEngine,
		Queries:         queries,
		ExternalURL:     externalURL,
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 5 * time.Minute, // FlareSolverr can take 60-120s per challenge solve
		IdleTimeout:  120 * time.Second,
	}

	// ── Background services ─────────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Prowlarr catalog refresh — fetch live definitions on startup, then daily.
	// After each refresh, load the raw YAML into the scraper engine.
	go func() {
		prowlarrCatalog.StartRefreshLoop(ctx, 24*time.Hour)
	}()
	// Wait a moment for the initial fetch, then load definitions into the engine.
	go func() {
		// Poll until the catalog has raw YAML data (initial fetch takes a few seconds).
		for i := 0; i < 30; i++ {
			time.Sleep(1 * time.Second)
			raw := prowlarrCatalog.AllRawYAML()
			if len(raw) > 0 {
				scraperEngine.LoadDefinitions(raw)

				// Pre-warm FlareSolverr sessions for configured indexers.
				if flaresolverr != nil {
					go func() {
						idxRows, err := queries.ListEnabledIndexers(ctx)
						if err != nil {
							logger.Warn("pre-warm: failed to list indexers", "error", err)
							return
						}
						var urls []string
						for _, row := range idxRows {
							if row.Url != "" {
								urls = append(urls, row.Url)
							}
						}
						if len(urls) > 0 {
							flaresolverr.PreWarm(ctx, urls)
						}
					}()
				}
				return
			}
		}
		logger.Warn("scraper: timed out waiting for Prowlarr definitions")
	}()

	healthTicker := time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-healthTicker.C:
				healthChecker.CheckAll(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()

	// ── Start server ─────────────────────────────────────────────────────
	go func() {
		logger.Info("pulse listening", "addr", addr)
		logger.Info("API docs available", "url", fmt.Sprintf("http://%s/api/docs", addr))
		logger.Info("API key", "key", cfg.Auth.APIKey.Value())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// ── Graceful shutdown ────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")
	healthTicker.Stop()
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}
	logger.Info("pulse stopped")
}

// parseHostPort extracts host and port from a URL like "http://hostname:8484".
func parseHostPort(apiURL string) (string, int) {
	// Strip scheme.
	u := apiURL
	if idx := strings.Index(u, "://"); idx >= 0 {
		u = u[idx+3:]
	}
	// Strip path.
	if idx := strings.Index(u, "/"); idx >= 0 {
		u = u[:idx]
	}
	host, portStr, err := net.SplitHostPort(u)
	if err != nil {
		return u, 0
	}
	port, _ := strconv.Atoi(portStr)
	return host, port
}
