# Pulse: notes for Claude Code

Pulse is the Beacon stack's hub: a service registry, the shared indexers,
download clients and quality profiles, the Torznab proxy that scrapes
indexer sites, notifications, and the VPN panel. It listens on 9696.

## Working here

- Work on a branch and merge through a pull request. Never commit to `main`.
- `gh` commands target `Beacon-Stack/pulse`.
- `make check` before pushing: lint, dead-code checks, Go tests and the
  frontend build.
- Comments say what the code does and why, not how it got that way;
  history belongs in the commit message.
- SQL lives in `internal/db/queries/sqlite/`; run `make sqlc` after
  changing it. Don't edit `internal/db/generated/`.
- `web/ui/src/shared/` is a copy of the `web-shared` repo. Change it
  there, not here.
- The Docker image builds with the Go version in the `Dockerfile`. After
  `go get`, check the `go` line in `go.mod` didn't move past it.

## Layout

```
cmd/pulse/            entry point and wiring
pkg/                  packages the other apps import (see below)
internal/api/         HTTP routes (huma + chi), middleware, WebSocket hub
internal/core/        registry, indexer, downloadclient, qualityprofile,
                      sharedsettings, health, gluetun (VPN), dashboard, tag
internal/scraper/     runs indexer definitions against sites
internal/torznab/     the Torznab feed the apps search through
internal/sitenet/     outbound requests to indexer sites (VPN, FlareSolverr)
internal/db/          migrations, queries, generated code
web/ui/               React + TypeScript + Vite; built into web/static/
```

## `pkg/` is a public API

Pilot, Prism and Haul import `pkg/sdk`, `pkg/notify`, `pkg/healthcheck`
and `pkg/log` at a pinned commit of this module.

- A function in `pkg/` that nothing in Pulse calls is not dead: check the
  other three repos before removing or changing it. The dead-code check
  in CI skips `pkg/` for this reason.
- A change the apps need only reaches them when each one bumps its Pulse
  version in `go.mod`.

## Indexer catalog

- Catalog entries come from the Prowlarr definitions: fetched from GitHub
  at start and daily, with `internal/core/indexer/prowlarr_snapshot.zip`
  as the offline copy (`make prowlarr-snapshot` refreshes it).
- `internal/core/indexer/catalog_data.go` holds the built-in entries.
