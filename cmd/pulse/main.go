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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/beacon-stack/pulse/pkg/auth"
	"github.com/beacon-stack/pulse/pkg/healthcheck"
	"github.com/beacon-stack/pulse/pkg/notify"

	"github.com/beacon-stack/pulse/internal/api"
	"github.com/beacon-stack/pulse/internal/api/ws"
	"github.com/beacon-stack/pulse/internal/config"
	cfgstore "github.com/beacon-stack/pulse/internal/core/config"
	"github.com/beacon-stack/pulse/internal/core/dashboard"
	"github.com/beacon-stack/pulse/internal/core/dockerstats"
	"github.com/beacon-stack/pulse/internal/core/downloadclient"
	"github.com/beacon-stack/pulse/internal/core/gluetun"
	"github.com/beacon-stack/pulse/internal/core/health"
	"github.com/beacon-stack/pulse/internal/core/indexer"
	"github.com/beacon-stack/pulse/internal/core/indexerprobe"
	"github.com/beacon-stack/pulse/internal/core/qualityprofile"
	"github.com/beacon-stack/pulse/internal/core/registry"
	"github.com/beacon-stack/pulse/internal/core/sharedsettings"
	"github.com/beacon-stack/pulse/internal/core/tag"
	"github.com/beacon-stack/pulse/internal/db"
	dbgen "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/events"
	"github.com/beacon-stack/pulse/internal/scraper"
	"github.com/beacon-stack/pulse/internal/sitenet"
	"github.com/beacon-stack/pulse/pkg/log"
	"github.com/beacon-stack/pulse/pkg/log/plugins/file"
	"github.com/beacon-stack/pulse/pkg/log/plugins/loki"
)

func main() {
	// "pulse healthcheck" (and the wget link) answer the container health
	// check and exit; the image has no shell or wget of its own.
	healthcheck.Run(os.Args, "http://127.0.0.1:"+envOr("PULSE_SERVER_PORT", "9696")+"/health")
	configPath := flag.String("config", "", "path to config.yaml")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pulse: %v\n", err)
		os.Exit(1)
	}

	// Logger
	// Shared module from pulse/pkg/log: gives us stdout JSON, the
	// in-memory ring buffer behind /api/v1/system/logs, and pluggable
	// sinks (Loki, file, …). The stdout handler is always on; plugins
	// are opt-in via env vars below.
	logger, logSystem := log.New(log.Config{
		Service: "pulse",
		Level:   cfg.Log.Level,
		Format:  cfg.Log.Format,
	})
	slog.SetDefault(logger)
	defer logSystem.Close(context.Background())

	// Optional plugin sinks. Failures are logged + skipped (logger
	// keeps working with just stdout) — a misconfigured Loki URL
	// must not stop Pulse from booting.
	if url := os.Getenv("BEACON_LOG_LOKI_URL"); url != "" {
		p, err := loki.New(loki.Config{
			Service:   "pulse",
			URL:       url,
			TenantID:  os.Getenv("BEACON_LOG_LOKI_TENANT"),
			BasicUser: os.Getenv("BEACON_LOG_LOKI_USER"),
			BasicPass: os.Getenv("BEACON_LOG_LOKI_PASS"),
		})
		if err != nil {
			logger.Warn("loki plugin disabled", "error", err)
		} else {
			logSystem.Add(p)
			logger.Info("log sink: loki", "url", url)
		}
	}
	if path := os.Getenv("BEACON_LOG_FILE_PATH"); path != "" {
		p, err := file.New(file.Config{Path: path})
		if err != nil {
			logger.Warn("file log plugin disabled", "error", err)
		} else {
			logSystem.Add(p)
			logger.Info("log sink: file", "path", path)
		}
	}

	// Docker socket reader: the in-app /api/v1/system/logs/docker
	// endpoint serves much more history than the ring buffer. Falls
	// back gracefully when the socket isn't mounted.
	dockerLogs := log.NewDockerLogsReader()

	logger.Info("starting pulse",
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
		"db_path", cfg.Database.Path,
	)

	if cfg.ConfigFile != "" {
		logger.Info("loaded config file", "path", cfg.ConfigFile)
	}

	// Database
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

	// Event bus
	bus := events.New(logger)

	// SQLC queries
	queries := dbgen.New(database.SQL)

	// Notifications: Pulse broadcasts the apps' events on its channels.
	notifyStore, err := notify.NewStore(context.Background(), database.SQL)
	if err != nil {
		logger.Error("notifications unavailable", "error", err)
		os.Exit(1)
	}
	notifyOutbox := &notify.Outbox{App: "Pulse", Store: notifyStore, Log: logger}
	// Announce indexers that start failing (and recover), on Pulse's own
	// channels.
	go (&notify.HealthWatcher{Outbox: notifyOutbox, Check: func(ctx context.Context) []notify.Problem {
		rows, err := queries.ListIndexers(ctx)
		if err != nil {
			return nil
		}
		var out []notify.Problem
		for _, r := range rows {
			if !r.Enabled || (r.HealthStatus != "failing" && r.HealthStatus != "definition_missing") {
				continue
			}
			msg := "Indexer " + r.Name + " is failing"
			if r.HealthStatus == "definition_missing" {
				msg = "Indexer " + r.Name + " no longer has a definition and can't be searched"
			} else if r.LastError != "" {
				msg += ": " + r.LastError
			}
			out = append(out, notify.Problem{Key: "indexer:" + r.ID, Message: msg, Level: notify.LevelWarning})
		}
		return out
	}}).Run(context.Background())

	// Core services
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

	// Prowlarr-sourced indexer catalog
	prowlarrCatalog := indexer.NewProwlarrCatalog(logger)
	indexer.SetCatalogSource(prowlarrCatalog.Entries)

	// Indexer site traffic (optional proxy)
	if err := sitenet.SetProxy(cfg.Scraper.ProxyURL); err != nil {
		logger.Error("scraper proxy", "error", err)
		os.Exit(1)
	}
	if p := sitenet.Proxy(); p != nil {
		logger.Info("indexer site requests go through proxy", "proxy", p.Redacted())
	}

	// FlareSolverr (optional)
	flaresolverr := scraper.NewFlareSolverr(cfg.FlareSolverr.URL, logger)
	if flaresolverr != nil {
		logger.Info("FlareSolverr configured", "url", cfg.FlareSolverr.URL)
	}

	// Scraper engine
	scraperEngine := scraper.NewEngine(logger, flaresolverr)
	catalogProber := indexerprobe.New(queries, scraperEngine, indexer.Catalog, logger)

	// WebSocket hub
	wsHub := ws.NewHub(logger)
	bus.Subscribe(wsHub.HandleEvent)

	// HTTP router
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

	// Optional dashboard integrations — both default off and degrade gracefully.
	dockerClient, err := dockerstats.NewClient(cfg.Dashboard.DockerSocket, cfg.Dashboard.ContainerNames)
	if err != nil {
		logger.Warn("dashboard: docker stats disabled", "error", err)
	} else if dockerClient != nil {
		logger.Info("dashboard: docker stats enabled", "socket", cfg.Dashboard.DockerSocket)
	}
	gluetunClient := gluetun.NewClient(cfg.Dashboard.GluetunURL, logger)
	if gluetunClient != nil {
		gluetunClient.SetAPIKey(cfg.Dashboard.GluetunAPIKey)
		logger.Info("dashboard: gluetun integration enabled", "url", cfg.Dashboard.GluetunURL)
	}
	dashAgg := dashboard.New(registrySvc, dockerClient, gluetunClient)
	if cfg.Dashboard.GluetunContainer != "" {
		dashAgg.GluetunService = cfg.Dashboard.GluetunContainer
	}

	// preWarmSessions solves Cloudflare challenges for all configured
	// indexers so the first user search doesn't pay the 60-120s solve
	// cost. Reused by the first catalog load and by VPN IP rotation
	// (which invalidates every session).
	preWarmSessions := func(warmCtx context.Context) {
		if flaresolverr == nil {
			return
		}
		idxRows, err := queries.ListEnabledIndexers(warmCtx)
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
			flaresolverr.PreWarm(warmCtx, urls)
		}
	}

	// VPN IP rotation — only when a gluetun control server is configured.
	var vpnRotator *gluetun.Rotator
	if gluetunClient != nil {
		vpnRotator = gluetun.NewRotator(gluetunClient, scraperEngine.ClearScraperState, preWarmSessions, bus, logger)
		if cfg.Dashboard.GluetunAPIKey != "" && cfg.Dashboard.GluetunServersDir != "" {
			vpnRotator.EnableRegions(gluetunClient,
				gluetun.RegionCatalog{Dir: cfg.Dashboard.GluetunServersDir},
				regionPreference{store: configStore})
			logger.Info("dashboard: VPN region changes enabled", "servers_dir", cfg.Dashboard.GluetunServersDir)
		}
	}

	// Who may use the app. Settings live in auth.json beside the database.
	gate, err := auth.Open(auth.Options{
		App:           "Pulse",
		APIKey:        cfg.Auth.APIKey.Value(),
		AllowedHosts:  auth.ListFromEnv("PULSE_SERVER_ALLOWED_HOSTS"),
		LocalNetworks: auth.ListFromEnv("PULSE_AUTH_LOCAL_NETWORKS"),
	}, filepath.Dir(cfg.Database.Path), auth.Settings{})
	if err != nil {
		logger.Error("failed to load login settings", "error", err)
		os.Exit(1)
	}
	switch gate.Settings().Mode() {
	case auth.RequiredEverywhere:
		logger.Info("a login is required from every address")
	case auth.RequiredOutside:
		logger.Info("the local network needs no login; other addresses log in")
	default:
		logger.Info("no login: the local network gets in, other addresses are refused")
	}

	router := api.NewRouter(api.RouterConfig{
		Auth:                  cfg.Auth.APIKey,
		Gate:                  gate,
		Logger:                logger,
		StartTime:             startTime,
		DBPath:                cfg.Database.Path,
		NotifyStore:           notifyStore,
		NotifyOutbox:          notifyOutbox,
		RegistryService:       registrySvc,
		IndexerManager:        indexerMgr,
		CatalogProber:         catalogProber,
		ConfigStore:           configStore,
		TagService:            tagSvc,
		DownloadClientService: dlClientSvc,
		QualityProfileService: qualityProfileSvc,
		SharedSettingsService: sharedSettingsSvc,
		WSHub:                 wsHub,
		ScraperEngine:         scraperEngine,
		Queries:               queries,
		DashboardAggregator:   dashAgg,
		VPNRotator:            vpnRotator,
		ExternalURL:           externalURL,
		LogSystem:             logSystem,
		DockerLogs:            dockerLogs,
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 5 * time.Minute, // FlareSolverr can take 60-120s per challenge solve
		IdleTimeout:  120 * time.Second,
	}

	// Background services
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Prowlarr definitions come from three sources, freshest wins: the
	// snapshot embedded at build time, the on-disk cache of the last
	// successful GitHub fetch, and the live GitHub fetch (daily). The local
	// sources make search work from first boot even when GitHub is
	// unreachable (e.g. broken container DNS); the live fetch only freshens.
	prowlarrCatalog.SetCachePath(filepath.Join(filepath.Dir(cfg.Database.Path), "prowlarr-definitions.zip"))
	var preWarmOnce sync.Once
	prowlarrCatalog.SetUpdateFunc(func(raw map[string][]byte) {
		scraperEngine.LoadDefinitions(raw)

		// Cross-check configured indexers against the fresh catalog so a
		// definition removed upstream is flagged (and withheld from
		// services) here, not discovered by failing user searches.
		go indexerMgr.AuditCatalog(ctx, scraperEngine.ResolveCatalogID)

		// Pre-warm FlareSolverr sessions for configured indexers, once the
		// first definition set is in place.
		preWarmOnce.Do(func() { go preWarmSessions(ctx) })
	})
	if source, err := prowlarrCatalog.LoadLocal(); err != nil {
		logger.Warn("prowlarr: no local definitions available, search is down until the GitHub fetch succeeds", "error", err)
	} else {
		logger.Info("prowlarr: local definitions loaded", "source", source)
	}
	go func() {
		prowlarrCatalog.StartRefreshLoop(ctx, 24*time.Hour)
	}()

	// Put the user's chosen VPN region back if the VPN container restarts
	// (its SERVER_REGIONS environment applies again on boot).
	if vpnRotator != nil {
		go vpnRotator.KeepPreferredRegion(ctx, 2*time.Minute)
	}

	// Keep FlareSolverr sessions perpetually warm. Sessions expire after
	// 25 minutes and a cold solve takes 60-120s — far beyond what an
	// interactive search's timeout can absorb — so re-solve on a cadence
	// inside the refresh margin instead of on demand. A pass over
	// already-fresh sessions is free (PreWarm skips them).
	if flaresolverr != nil {
		go func() {
			warmTicker := time.NewTicker(10 * time.Minute)
			defer warmTicker.Stop()
			for {
				select {
				case <-warmTicker.C:
					preWarmSessions(ctx)
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Catalog tester: test-search public indexers that haven't been added,
	// every 6 hours, so the add-indexer page can recommend ones that work.
	go catalogProber.Run(ctx)

	// Per-indexer search stats: finalize cross-indexer comparisons and
	// prune rows older than 30 days.
	go indexerMgr.Stats().Run(ctx)

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

	// Start server
	go func() {
		logger.Info("pulse listening", "addr", addr)
		logger.Info("API docs available", "url", fmt.Sprintf("http://%s/api/docs", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
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

// envOr returns the environment variable, or def when it's unset.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
