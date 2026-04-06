# Plan: Shared Configuration Management

## Context

Pulse is the central control plane for the Arr ecosystem. Currently it manages indexers — now we're extending it to manage all shared configuration that multiple services need. Instead of configuring quality profiles, download clients, naming rules, and media management settings in each service separately, users configure them once in Pulse and they're pushed to all connected services.

## Scope

Four config domains, in build order:

1. **Download Clients** — qBittorrent, Deluge, Transmission, SABnzbd, NZBGet
2. **Quality Profiles** — resolution preferences, codec rankings, size limits
3. **Naming Conventions** — movie/TV file and folder naming templates
4. **Media Management** — root folders, permissions, hardlinks, recycling bin

## Architecture

Each config domain follows the same pattern:

```
User configures in Pulse UI
  ↓
Stored in Pulse's DB
  ↓
When a service registers (or config changes):
  Push to all subscribed services via webhook
  ↓
Service receives config → auto-creates/updates locally
```

This is the same pattern we already have for indexers. The SDK already supports config subscriptions (`client.Subscribe(ctx, "quality")`) and the config store exists.

## Phase 1: Download Clients

### Why first
- Most requested feature (from our earlier discussion)
- Self-contained — doesn't depend on the other config types
- Immediately useful — eliminates the most common manual config step

### Data Model

```sql
CREATE TABLE download_clients (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL,       -- qbittorrent, deluge, transmission, sabnzbd, nzbget
    protocol    TEXT NOT NULL,       -- torrent, usenet
    enabled     INTEGER NOT NULL DEFAULT 1,
    priority    INTEGER NOT NULL DEFAULT 1,
    host        TEXT NOT NULL,       -- hostname or IP
    port        INTEGER NOT NULL,
    use_ssl     INTEGER NOT NULL DEFAULT 0,
    username    TEXT NOT NULL DEFAULT '',
    password    TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL DEFAULT '',  -- download category/label
    directory   TEXT NOT NULL DEFAULT '',  -- optional download directory
    settings    TEXT NOT NULL DEFAULT '{}', -- JSON: kind-specific extra settings
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
```

### API Endpoints

```
POST   /api/v1/download-clients           — create
GET    /api/v1/download-clients           — list all
GET    /api/v1/download-clients/{id}      — get one
PUT    /api/v1/download-clients/{id}      — update
DELETE /api/v1/download-clients/{id}      — delete
POST   /api/v1/download-clients/{id}/test — test connectivity
```

### Frontend

New page: `/download-clients` with sidebar nav entry.
- Card-based list (same style as Indexers page)
- Add Download Client modal/drawer with kind selector
- Per-kind config fields (qBit needs host/port/user/pass, SABnzbd needs host/port/apikey, etc.)
- Test button that verifies connectivity
- Edit/delete on detail page

### Push to Services

When a download client is created/updated/deleted:
1. Find all services that declare `supports_torrent` (for torrent clients) or `supports_usenet` (for usenet clients)
2. POST to each service's sync webhook
3. Service receives the download client config and creates it locally

### SDK Extension

Add to `pkg/sdk/client.go`:
```go
type DownloadClient struct {
    ID       string
    Name     string
    Kind     string
    Protocol string
    Host     string
    Port     int
    UseSSL   bool
    Username string
    Password string
    Category string
    Settings string // JSON
}

func (c *Client) MyDownloadClients(ctx context.Context) ([]DownloadClient, error)
```

### Luminarr Integration

Add download client sync to `internal/pulse/sync.go`:
- Pull download clients from Pulse on startup
- Create/update/delete in Luminarr's local DB (same pattern as indexer sync)
- Match by name to detect existing vs new

## Phase 2: Quality Profiles

### Data Model

```sql
CREATE TABLE quality_profiles (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    cutoff      TEXT NOT NULL,           -- quality ID where upgrading stops
    items       TEXT NOT NULL,           -- JSON array of quality items with allowed/preferred
    min_size    REAL NOT NULL DEFAULT 0, -- MB per minute
    max_size    REAL NOT NULL DEFAULT 0,
    upgrade     INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
```

### Quality Items Format
```json
[
    {"quality": "Bluray-2160p", "allowed": true, "preferred": true},
    {"quality": "Bluray-1080p", "allowed": true, "preferred": false},
    {"quality": "WEB-DL 1080p", "allowed": true, "preferred": false},
    {"quality": "HDTV-720p", "allowed": false, "preferred": false}
]
```

### Frontend
- Quality Profiles page with drag-to-reorder quality rankings
- Create/edit modal with quality checkboxes and size sliders
- Cutoff selector dropdown

## Phase 3: Naming Conventions

### Data Model

Store as config entries with namespace `naming`:
- `movie_format` — e.g., `{Title} ({Year}) [{Quality} {Codec}]`
- `movie_folder` — e.g., `{Title} ({Year})`
- `tv_format` — e.g., `{Series} S{Season:00}E{Episode:00} - {Title}`
- `tv_folder` — e.g., `{Series}/Season {Season}`
- `replace_spaces` — boolean
- `colon_replacement` — e.g., " -" or ""

### Frontend
- Naming page with live preview of each template
- Token picker for available variables

## Phase 4: Media Management

### Data Model

Store as config entries with namespace `media`:
- `root_folders` — JSON array of paths
- `permissions_chmod` — e.g., "755"
- `permissions_chown` — e.g., "1000:1000"
- `use_hardlinks` — boolean
- `recycling_bin` — path or empty
- `import_mode` — "move" or "copy"

### Frontend
- Media Management page with folder browser
- Permission settings
- Import mode toggle

## Files to Create/Modify (Phase 1 only)

### New Files
| File | Purpose |
|------|---------|
| `internal/db/migrations/00003_download_clients.sql` | DB schema |
| `internal/db/queries/sqlite/download_clients.sql` | SQLC queries |
| `internal/core/downloadclient/service.go` | CRUD service + test |
| `internal/api/v1/download_clients.go` | API handlers |
| `web/ui/src/pages/download-clients/DownloadClientsPage.tsx` | List page |
| `web/ui/src/pages/download-clients/AddDownloadClient.tsx` | Add modal/drawer |
| `web/ui/src/pages/download-clients/DownloadClientDetail.tsx` | Detail page |
| `web/ui/src/api/download-clients.ts` | React Query hooks |

### Modified Files
| File | Change |
|------|--------|
| `internal/api/router.go` | Register download client routes |
| `cmd/pulse/main.go` | Wire download client service |
| `web/ui/src/App.tsx` | Add routes |
| `web/ui/src/layouts/Shell.tsx` | Add sidebar nav item |
| `pkg/sdk/client.go` | Add MyDownloadClients() |
| `internal/core/indexer/pusher.go` | Reuse push pattern for download clients |

## Verification (Phase 1)

1. Add qBittorrent in Pulse UI with host/port/credentials
2. Click Test — verify connectivity
3. Start Luminarr — download client auto-synced
4. Check Luminarr Settings → Download Clients — qBittorrent appears with Pulse badge
5. Update credentials in Pulse — Luminarr updates within 30s
6. Delete in Pulse — removed from Luminarr
