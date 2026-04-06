# Configurarr

Centralized control plane for the Arr ecosystem — service registry, discovery, shared config, and indexer management.

## Build & Run

```bash
make build          # compile binary to bin/configurarr
make run            # build + run
make dev            # hot-reload with air
make sqlc           # regenerate SQLC code from SQL queries
make test           # run all tests
make docker         # build Docker image
```

### Frontend

```bash
cd web/ui
npm install
npm run dev         # Vite dev server with proxy to :9696
npm run build       # production build → web/static/
```

## Project Structure

```
cmd/configurarr/        main entry point
pkg/sdk/                Go client SDK for ecosystem services
internal/
  api/                  HTTP layer (Huma v2 + Chi v5)
    v1/                 API v1 handlers
    middleware/          auth, security, logging, recovery
    ws/                 WebSocket hub for event streaming
  core/
    registry/           service registration & discovery
    config/             shared config key-value store
    indexer/            centralized indexer management + catalog + tester
    health/             health-check poller
    tag/                tag management
  db/
    migrations/         goose SQL migrations
    queries/sqlite/     SQLC query definitions
    generated/sqlite/   auto-generated Go code (DO NOT EDIT)
  events/               in-process event bus
  config/               app configuration (Viper)
web/
  ui/                   React 19 + TypeScript + Vite + TailwindCSS frontend
  static/               built frontend assets (embedded in binary)
  embed.go              Go embed for serving the SPA
```

## Tech Stack

- **Go 1.25** with Huma v2 (OpenAPI-first) + Chi v5 router
- **SQLite** (WAL mode) via SQLC for type-safe queries
- **Goose** for database migrations
- **Viper** for config (YAML + env vars `CONFIGURARR_*`)
- **WebSocket** (coder/websocket) for real-time event streaming
- **React 19** + TypeScript + Vite + TailwindCSS v4 frontend
- **React Query** for server state, **Sonner** for toasts

## Key Conventions

- All IDs are UUIDs (string type)
- Timestamps are RFC3339 UTC strings
- API auth: X-Api-Key header (auto-generated on first run)
- Same-origin browser requests trusted via Sec-Fetch-Site
- Event bus is non-blocking, handlers run in goroutines
- SQLC generated code lives in `internal/db/generated/sqlite/` — never edit directly
- Config supports env var overrides: `CONFIGURARR_SERVER_PORT=8080`
- Frontend uses inline CSS with CSS custom properties for theming (same as Luminarr)
- Catalog entries live in `internal/core/indexer/catalog_data.go` — add new indexers there

## Client SDK

The `pkg/sdk` package lets any Go service integrate with Configurarr:

```go
client, err := sdk.New(sdk.Config{
    ConfigurarURL: "http://configurarr:9696",
    APIKey:        "your-api-key",
    ServiceName:   "luminarr",
    ServiceType:   "media-manager",
    APIURL:        "http://luminarr:8282",
    Capabilities:  []string{"supports_torrent"},
})
defer client.Close()

// Discover download clients
downloaders, _ := client.DiscoverByType(ctx, "download-client")

// Read shared config
codec, _ := client.GetConfig(ctx, "quality", "preferred_codec")

// Get indexers assigned to this service
indexers, _ := client.MyIndexers(ctx)
```

## API Endpoints

All endpoints are under `/api/v1/`. Interactive docs at `/api/docs`.

### Services
- `POST /services/register` — register/re-register
- `GET /services` — list all
- `GET /services/discover?type=...&capability=...` — discover
- `GET /services/{id}` — get one
- `PUT /services/{id}/heartbeat` — heartbeat
- `DELETE /services/{id}` — deregister

### Config
- `PUT /config` — set entry (upsert)
- `GET /config` — list all entries
- `GET /config/namespaces` — list namespaces
- `GET /config/{namespace}` — list by namespace
- `GET /config/{namespace}/{key}` — get one
- `DELETE /config/{namespace}/{key}` — delete
- `POST /config/subscribe` — subscribe service to namespace
- `POST /config/unsubscribe` — unsubscribe
- `GET /config/subscriptions/{service_id}` — list subscriptions

### Indexers
- `POST /indexers` — create
- `GET /indexers` — list all
- `GET /indexers/{id}` — get one
- `PUT /indexers/{id}` — update
- `DELETE /indexers/{id}` — delete
- `POST /indexers/{id}/assign` — assign to service
- `DELETE /indexers/{id}/assign/{service_id}` — unassign
- `GET /indexers/{id}/assignments` — list assignments
- `GET /services/{service_id}/indexers` — indexers for a service
- `POST /indexers/test` — test indexer connectivity

### Catalog
- `GET /indexers/catalog?q=...&protocol=...&privacy=...&category=...` — browse catalog
- `GET /indexers/catalog/{id}` — single catalog entry with settings schema

### Tags
- `GET /tags` — list all with usage counts
- `POST /tags` — create
- `PUT /tags/{id}` — rename
- `DELETE /tags/{id}` — delete

### System
- `GET /system/status` — status & uptime
- `GET /health` — container health check (no auth)
- `WS /ws` — WebSocket event stream

## Frontend Pages

- `/` — Dashboard with stats and service overview
- `/services` — Service registry with register/deregister
- `/indexers` — Managed indexers with assign-to-service
- `/indexers/add` — Add Indexer catalog browser (search, filter, grid/list, config drawer)
- `/config` — Shared config store with namespace filtering
- `/settings/system` — System info
- `/settings/app` — Theme picker (6 dark + 1 light preset)

## Default Port

9696 (configurable via `CONFIGURARR_SERVER_PORT` or `config.yaml`)
