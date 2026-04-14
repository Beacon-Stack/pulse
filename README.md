<p align="center">
  <h1 align="center">Pulse</h1>
  <p align="center">The control plane for the Beacon media stack.</p>
</p>
<p align="center">
  <a href="https://github.com/beacon-stack/pulse/blob/main/LICENSE"><img src="https://img.shields.io/github/license/beacon-stack/pulse" alt="License"></a>
  <img src="https://img.shields.io/badge/go-1.25-00ADD8?logo=go&logoColor=white" alt="Go 1.25">
</p>
<p align="center">
  <a href="https://beaconstack.io">Website</a> ·
  <a href="https://github.com/beacon-stack/pulse/issues">Bug Reports</a>
</p>

---

**Pulse** is the central service that every other Beacon application — [Pilot](https://github.com/beacon-stack/pilot), [Prism](https://github.com/beacon-stack/prism), [Haul](https://github.com/beacon-stack/haul) — registers with and pulls shared configuration from. If you run more than one Beacon service, Pulse is what stops you from editing the same indexer list, the same quality profile, and the same download-client credentials in every service separately.

Pulse is optional for a single-service install. It becomes essential the moment you add the second one.

## What Pulse does

**Service registry**

Each Beacon service registers itself with Pulse on startup (name, type, URL, capabilities) and heartbeats periodically. Other services discover their peers through Pulse instead of hardcoding URLs. If you move Pilot to a new host, Prism picks up the new address on the next heartbeat cycle — no config edits.

**Indexer management**

Indexers are configured once in Pulse and pushed out to subscribing services. Add a new Torznab endpoint to Pulse, tag it with `movies`, and Prism picks it up automatically. Tag it with `tv` and Pilot picks it up too. No more adding the same indexer to three services and keeping three copies of its API key in sync.

Includes a built-in catalog of popular indexers with pre-filled setting schemas and a one-click add flow, plus a tester that verifies connectivity and returns sample results before you save.

**Quality profile management**

Quality profiles live in Pulse. Subscribing services mirror them with a `managed_by_pulse` flag so local edits are clearly distinguished from synced state. If you want a 1080p-cutoff profile identical in Pilot and Prism, you edit it once in Pulse.

**Shared settings store**

A typed key-value store for cross-service configuration. Things like `media.library_root`, `rename.colon_replacement`, `download.preferred_protocol` — settings multiple services need to agree on. Each service can read, write, and subscribe to a namespace.

**Tag management**

Central tag registry with usage counts. Tags are used by indexer assignment, notification routing, and library filtering across every service.

## Features

- Full REST API (Huma v2 + Chi v5) under `/api/v1/`
- OpenAPI documentation at `/api/docs`
- WebSocket event stream at `/ws` — services subscribe for live updates when shared state changes
- Postgres-backed state
- Go client SDK at `pkg/sdk` — drop-in for any Go service that wants to register with Pulse
- React 19 + TypeScript + Vite frontend embedded in the Go binary
- Per-mode theme system (dark + light presets) shared with the other Beacon services
- Dashboard with service overview, indexer status, and config subscriptions
- Zero telemetry

## Getting started

### Docker Compose (recommended, as part of the Beacon stack)

Pulse is designed to run alongside the rest of the Beacon stack. The full `docker-compose.yml` with Postgres, Pulse, Pilot, Prism, and Haul lives in [`beacon-stack/stack`](https://github.com/beacon-stack/stack).

### Standalone Docker

```bash
docker run -d \
  --name pulse \
  -p 9696:9696 \
  -v /path/to/config:/config \
  ghcr.io/beacon-stack/pulse:latest
```

Open `http://localhost:9696`.

### Build from source

Requires Go 1.25+ and Node.js 22+.

```bash
git clone https://github.com/beacon-stack/pulse
cd pulse
cd web/ui && npm ci && npm run build && cd ../..
make build
./bin/pulse
```

## Configuration

Pulse works with zero configuration. All settings are editable through the web UI or via environment variables.

### Key environment variables

| Variable | Default | Description |
|---|---|---|
| `PULSE_SERVER_HOST` | `0.0.0.0` | Bind address |
| `PULSE_SERVER_PORT` | `9696` | HTTP port |
| `PULSE_DATABASE_DSN` | | Postgres connection string |
| `PULSE_AUTH_API_KEY` | auto-generated | API key for external access |
| `PULSE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `PULSE_LOG_FORMAT` | `json` | `json` or `text` |

### Config file

Pulse looks for `config.yaml` in `/config/config.yaml`, `~/.config/pulse/config.yaml`, `/etc/pulse/config.yaml`, or `./config.yaml` (in that order).

## Using the SDK

Any Go service can integrate with Pulse using `pkg/sdk`:

```go
import "github.com/beacon-stack/pulse/pkg/sdk"

client, err := sdk.New(sdk.Config{
    PulseURL:     "http://pulse:9696",
    APIKey:       "your-api-key",
    ServiceName:  "pilot",
    ServiceType:  "media-manager",
    APIURL:       "http://pilot:8383",
    Capabilities: []string{"supports_torrent", "supports_usenet"},
})
defer client.Close()

// Discover download clients
downloaders, _ := client.DiscoverByType(ctx, "download-client")

// Read shared config
root, _ := client.GetConfig(ctx, "media", "library_root")

// Get indexers assigned to this service
indexers, _ := client.MyIndexers(ctx)

// Subscribe to config changes — callback fires on WebSocket events
client.Subscribe(ctx, "media", func(ns, key, value string) {
    // ...
})
```

## Where Pulse fits in the Beacon stack

```
                   ┌─────────────┐
                   │    Pulse    │
                   │  (registry, │
                   │   indexers, │
                   │  profiles,  │
                   │   shared    │
                   │    config)  │
                   └──┬───┬───┬──┘
          registers ──┘   │   └── registers
          + pulls         │       + pulls
               ┌──────────┴──────────┐
          ┌────▼────┐           ┌────▼────┐
          │  Pilot  │           │  Prism  │
          │  (TV)   │           │ (Movies)│
          └────┬────┘           └────┬────┘
               │                     │
               └──────────┬──────────┘
                          ▼
                     ┌─────────┐
                     │  Haul   │
                     │  (BT)   │
                     └─────────┘
```

Pilot and Prism both register with Pulse and pull their indexers, quality profiles, and shared settings from it. Haul doesn't register but can optionally read shared config (for rename format, etc.) through the SDK.

## Privacy

Pulse makes outbound connections only to the indexers you configure (to test connectivity) and to the services that register with it. No telemetry, no analytics, no crash reporting, no update checks. API keys and indexer credentials are stored in your local database only.

## Project structure

```
cmd/pulse/                Entry point
pkg/sdk/                  Go client SDK for ecosystem services
internal/
  api/                    HTTP router, middleware, v1 handlers, WebSocket hub
  core/
    registry/             Service registration & discovery
    sharedsettings/       Shared config key-value store
    qualityprofile/       Quality profile CRUD + sync
    indexer/              Indexer management, catalog, tester, autoassign
    downloadclient/       Download client config CRUD
    health/               Health-check poller
    tag/                  Tag management
  db/                     Migrations and generated query code (sqlc)
  events/                 In-process event bus
  scraper/                Torznab proxy engine
  config/                 App configuration (Viper)
web/
  embed.go                Go embed for serving the SPA
  static/                 Built frontend assets
  ui/                     React 19 + TypeScript + Vite source
```

## Development

```bash
make build         # compile binary to bin/pulse
make run           # build + run
make dev           # hot reload with air
make test          # go test ./... -v
make sqlc          # regenerate SQLC code from queries
```

## Contributing

Bug reports, feature requests, and pull requests are welcome. Please open an issue before starting large changes.

## License

MIT
