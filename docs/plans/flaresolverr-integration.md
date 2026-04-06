# Plan: FlareSolverr Integration for Pulse

## Context

53 of 548 Prowlarr indexer definitions reference FlareSolverr (including popular ones like 1337x). When Pulse's scraper engine fetches these sites, Cloudflare returns a 403 challenge page instead of results. The scraper currently detects this (`CloudflareError`) and reports "Blocked by Cloudflare — requires FlareSolverr" but can't bypass it.

FlareSolverr is a proxy server that runs a headless browser (Selenium/Chrome) to solve Cloudflare challenges, then returns the HTML and session cookies. Once we have the cookies, subsequent direct HTTP requests work without the browser.

## FlareSolverr API

**Endpoint:** `POST http://flaresolverr:8191/v1`

**Request:**
```json
{
  "cmd": "request.get",
  "url": "https://1337x.to/search/inception/1/",
  "maxTimeout": 60000
}
```

**Response:**
```json
{
  "status": "ok",
  "solution": {
    "url": "https://1337x.to/search/inception/1/",
    "status": 200,
    "response": "<html>...full page HTML...</html>",
    "cookies": [
      {"name": "cf_clearance", "value": "abc123", "domain": "1337x.to", ...}
    ],
    "userAgent": "Mozilla/5.0 ..."
  }
}
```

The key insight: **we don't need FlareSolverr for every request.** We use it once to get the `cf_clearance` cookie + user agent, then set those on our normal HTTP client. The cookie is valid for ~30 minutes, so one FlareSolverr call per indexer per ~30 min.

## Architecture

```
Runner.executePath()
  ↓ send normal HTTP request
  ↓ got 403 + Cloudflare detected?
  ↓ YES → is FlareSolverr configured?
    ↓ YES → call FlareSolverr with the same URL
    ↓       extract cookies + user-agent from response
    ↓       set cookies on our HTTP client's cookie jar
    ↓       use the HTML response directly (no need to re-fetch)
    ↓       cache the cookies for subsequent requests
    ↓ NO  → return CloudflareError (current behavior)
```

This is a **transparent fallback** — the runner tries the direct request first, and only falls back to FlareSolverr when Cloudflare blocks it. No code changes needed for indexers that don't use Cloudflare.

## Implementation

### Phase 1: Config + FlareSolverr Client

**Modify:** `internal/config/config.go`
- Add `FlareSolverr` field to Config struct:
  ```go
  type FlareSolverrConfig struct {
      URL string `mapstructure:"url"` // e.g., "http://localhost:8191"
  }
  ```

**Modify:** `internal/config/load.go`
- No default for FlareSolverr URL — disabled when empty

**New file:** `internal/scraper/flaresolverr.go`
- FlareSolverr HTTP client:
  ```go
  type FlareSolverr struct {
      url    string
      client *http.Client
      logger *slog.Logger

      // Cookie cache: domain → {cookies, userAgent, expiry}
      mu       sync.RWMutex
      sessions map[string]*cfSession
  }

  type cfSession struct {
      cookies   []*http.Cookie
      userAgent string
      created   time.Time
  }

  func (f *FlareSolverr) Solve(ctx context.Context, targetURL string) (html string, cookies []*http.Cookie, userAgent string, err error)
  func (f *FlareSolverr) HasSession(domain string) bool
  func (f *FlareSolverr) ApplySession(domain string, jar http.CookieJar, req *http.Request)
  ```
- `Solve()` sends `POST /v1` with `cmd: request.get`, parses response
- Sessions cached by domain for 25 minutes (Cloudflare cookies last ~30 min)
- `ApplySession()` sets cached cookies on the jar and user-agent on the request header

### Phase 2: Integrate into Runner

**Modify:** `internal/scraper/runner.go`
- Add `flaresolverr *FlareSolverr` field to Runner struct
- In `executePath()`, after detecting CloudflareError:
  1. Check if FlareSolverr is available (`r.flaresolverr != nil`)
  2. Check if we already have a cached session for this domain → apply and retry
  3. If no session, call `f.Solve(ctx, fullURL)` to get HTML + cookies
  4. Cache the session, apply cookies to the HTTP client
  5. **Use the HTML from the FlareSolverr response directly** (don't re-fetch)
  6. Parse the HTML as usual with goquery

**Modify:** `internal/scraper/httpclient.go`
- Add method to set cookies and user-agent from a FlareSolverr session:
  ```go
  func (c *RateLimitedClient) ApplyCFSession(domain string, cookies []*http.Cookie, userAgent string)
  ```

### Phase 3: Wire into Engine + Main

**Modify:** `internal/scraper/engine.go`
- Accept `*FlareSolverr` in constructor, pass to runners
- `NewEngine(logger, flaresolverr *FlareSolverr)`

**Modify:** `cmd/pulse/main.go`
- Read FlareSolverr config
- Create `FlareSolverr` client if URL is configured
- Pass to engine

**Modify:** `internal/api/router.go`
- No changes needed — FlareSolverr is internal to the scraper

### Phase 4: Frontend — Settings + Status

**Modify:** `web/ui/src/pages/settings/system/SystemPage.tsx`
- Show FlareSolverr status: configured URL, connectivity check
- Could add a "Test FlareSolverr" button

**Modify:** `internal/api/v1/system.go`
- Add FlareSolverr status to system status response (configured, reachable)

**Modify:** `config.example.yaml`
- Add FlareSolverr section with commented-out URL

### Phase 5: Docker Compose

**Modify:** `docker-compose.yml`
- Add FlareSolverr service:
  ```yaml
  flaresolverr:
    image: ghcr.io/flaresolverr/flaresolverr:latest
    ports:
      - "8191:8191"
    environment:
      LOG_LEVEL: info
    restart: unless-stopped
  ```

## Files Summary

| Action | File | Change |
|--------|------|--------|
| Create | `internal/scraper/flaresolverr.go` | FlareSolverr client + session cache |
| Modify | `internal/scraper/runner.go` | Cloudflare fallback → FlareSolverr retry |
| Modify | `internal/scraper/httpclient.go` | Apply CF session cookies/user-agent |
| Modify | `internal/scraper/engine.go` | Pass FlareSolverr to runners |
| Modify | `internal/config/config.go` | Add FlareSolverrConfig |
| Modify | `internal/config/load.go` | Wire FlareSolverr config |
| Modify | `cmd/pulse/main.go` | Create FlareSolverr client, pass to engine |
| Modify | `config.example.yaml` | Add flaresolverr section |
| Modify | `docker-compose.yml` | Add flaresolverr service |

## Config

```yaml
# config.yaml
flaresolverr:
  url: "http://localhost:8191"  # empty = disabled
```

Environment variable: `PULSE_FLARESOLVERR_URL=http://flaresolverr:8191`

## Verification

1. Start FlareSolverr: `docker run -d -p 8191:8191 ghcr.io/flaresolverr/flaresolverr:latest`
2. Configure: set `flaresolverr.url` in Pulse config
3. Add 1337x indexer in Pulse
4. Test it — should succeed instead of returning "Cloudflare blocked"
5. Check logs: should see "flaresolverr: solved challenge for 1337x.to" + "using cached CF session"
6. Second search should use cached cookies (no FlareSolverr call)
7. After 25 min, cache expires, next search calls FlareSolverr again

## Key Design Decisions

- **Transparent fallback:** Direct request first, FlareSolverr only on Cloudflare detection. No perf cost for non-CF sites.
- **Session caching:** One FlareSolverr call per domain per ~25 min. Cookie reuse avoids hammering FlareSolverr.
- **Use response HTML directly:** FlareSolverr already fetches the page — no need to re-fetch with cookies. Parse its HTML response with the same goquery selectors.
- **Optional dependency:** FlareSolverr is not required. Without it, CF-blocked indexers show the amber "Cloudflare blocked" indicator (current behavior).
