package v1

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/singleflight"

	"github.com/beacon-stack/pulse/internal/core/indexer"
	"github.com/beacon-stack/pulse/internal/core/indexerprobe"
	db "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/scraper"
	"github.com/beacon-stack/pulse/internal/sitenet"
	"github.com/beacon-stack/pulse/internal/torznab"
)

// TorznabHandler handles Torznab proxy requests.
type TorznabHandler struct {
	engine      *scraper.Engine
	q           db.Querier
	mgr         *indexer.Manager // nil-able; search-health outcome recording
	externalURL string           // e.g. "http://pulse:9696" — used for proxy download URLs in Docker
	logger      *slog.Logger
	sf          singleflight.Group // coalesces identical in-flight searches
}

// NewTorznabHandler creates a new handler. externalURL is optional; when set,
// torznab download proxy URLs use it instead of the request's Host header.
// mgr may be nil (tests) — search-health recording is then skipped.
func NewTorznabHandler(engine *scraper.Engine, q db.Querier, mgr *indexer.Manager, externalURL string, logger *slog.Logger) *TorznabHandler {
	return &TorznabHandler{engine: engine, q: q, mgr: mgr, externalURL: strings.TrimRight(externalURL, "/"), logger: logger}
}

// recordSearchOutcome forwards a search result to the health tracker.
// Client-side cancellations are not indexer failures and are ignored.
func (h *TorznabHandler) recordSearchOutcome(indexerID string, searchErr error) {
	if h.mgr == nil {
		return
	}
	if searchErr != nil && (errors.Is(searchErr, context.Canceled) || errors.Is(searchErr, context.DeadlineExceeded)) {
		return
	}
	go h.mgr.RecordSearchOutcome(context.Background(), indexerID, searchErr)
}

// recordSearchStats stores one real scrape in the per-indexer stats. A
// search cut short by shutdown is not the indexer's fault and is skipped;
// running out the background budget counts as a timeout.
func (h *TorznabHandler) recordSearchStats(indexerID, keywords, mode string, started time.Time, results []scraper.SearchResult, searchErr error, trace *scraper.Trace) {
	if h.mgr == nil || errors.Is(searchErr, context.Canceled) {
		return
	}
	seeders := make([]int, len(results))
	for i, res := range results {
		seeders[i] = res.Seeders
	}
	challenged, solved := trace.Challenge()
	ev := indexer.SearchEvent{
		IndexerID:  indexerID,
		At:         started,
		Duration:   time.Since(started),
		ErrorKind:  indexerprobe.ErrorKind(searchErr),
		Seeders:    seeders,
		Challenged: challenged,
		Solved:     solved,
		Query:      keywords,
		Mode:       indexerprobe.SearchMode(mode),
	}
	go h.mgr.Stats().RecordSearch(context.Background(), ev)
}

// noteResults hands a served answer to the cross-indexer overlap tracker.
func (h *TorznabHandler) noteResults(indexerID, keywords string, results []scraper.SearchResult) {
	if h.mgr == nil {
		return
	}
	ids := make([]indexer.ReleaseID, len(results))
	for i, res := range results {
		ids[i] = indexerprobe.ReleaseIDOf(res)
	}
	h.mgr.Stats().NoteResults(indexerID, strings.ToLower(strings.TrimSpace(keywords)), ids)
}

// RegisterTorznabRoutes registers the Torznab proxy endpoint on the chi router.
// This is registered directly on chi (not Huma) because Torznab uses XML, not JSON.
func RegisterTorznabRoutes(r chi.Router, h *TorznabHandler) {
	r.Get("/api/v1/torznab/{indexer_id}/api", h.Handle)
	r.Get("/api/v1/torznab/{indexer_id}/download", h.HandleDownload)
	r.Post("/api/v1/indexers/{indexer_id}/test-search", h.HandleTestSearch)
}

// Handle dispatches Torznab requests based on the ?t= parameter.
func (h *TorznabHandler) Handle(w http.ResponseWriter, r *http.Request) {
	indexerID := chi.URLParam(r, "indexer_id")
	t := r.URL.Query().Get("t")

	// Look up the indexer in the DB.
	idx, err := h.q.GetIndexer(r.Context(), indexerID)
	if err != nil {
		h.xmlError(w, 100, "Indexer not found")
		return
	}

	// The catalog_id is stored in the settings JSON or derived from the indexer name.
	// Resolve the Prowlarr catalog ID from the indexer name or URL.
	catalogID := h.engine.ResolveCatalogID(idx.Name, idx.Url)
	if catalogID == "" {
		h.xmlError(w, 100, fmt.Sprintf("No catalog definition found for indexer %q — it may have been removed from Prowlarr", idx.Name))
		return
	}

	runner, err := h.engine.GetRunner(catalogID, idx.Settings)
	if err != nil {
		h.logger.Warn("torznab: failed to get runner",
			"indexer", idx.Name, "catalog_id", catalogID, "error", err)
		h.xmlError(w, 100, "Failed to load indexer definition")
		return
	}

	switch t {
	case "caps":
		h.handleCaps(w, runner, idx)
	case "search", "tvsearch", "tv-search", "movie", "movie-search", "music", "book":
		h.handleSearch(w, r, runner, idx)
	default:
		h.xmlError(w, 201, fmt.Sprintf("Unknown function: %s", t))
	}
}

// handleCaps returns the Torznab capabilities document.
func (h *TorznabHandler) handleCaps(w http.ResponseWriter, runner *scraper.Runner, idx db.Indexer) {
	def := runner.Definition()

	// Build category list from the definition.
	catMap := make(map[string]*torznab.CapsCategory) // base cat → CapsCategory
	for _, cm := range def.Caps.CategoryMappings {
		parts := strings.SplitN(cm.Cat, "/", 2)
		baseName := parts[0]
		baseID := newznabBaseID(baseName)

		base, ok := catMap[baseID]
		if !ok {
			base = &torznab.CapsCategory{ID: baseID, Name: baseName}
			catMap[baseID] = base
		}

		if len(parts) > 1 {
			subID := baseID[:2] + cm.ID
			base.Subcats = append(base.Subcats, torznab.CapsCategory{
				ID: subID, Name: cm.Desc,
			})
		}
	}

	var cats []torznab.CapsCategory
	for _, c := range catMap {
		cats = append(cats, *c)
	}

	caps := torznab.NewCaps(idx.Name, cats, def.Caps.Modes)

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(caps)
}

// seasonEpisodeTokenRe detects an SxxEyy / Sxx / "Season N" token already
// present in a text query, so composeKeywords doesn't append a duplicate.
var seasonEpisodeTokenRe = regexp.MustCompile(`(?i)(?:^|\s)(?:S\d{1,2}(?:E\d{1,2})?|Season\s+\d{1,2})(?:\s|$)`)

// composeKeywords merges the torznab q/season/ep params into the keyword
// string a Cardigann definition searches with. Prowlarr does the same for
// definitions that only take free-text keywords: a Sonarr-style request
// `?t=tvsearch&q=Severance&season=2&ep=1` must search "Severance S02E01",
// not bare "Severance" (which returns every season, unfiltered).
func composeKeywords(q string, season, ep int) string {
	if q == "" || season <= 0 || seasonEpisodeTokenRe.MatchString(q) {
		return q
	}
	if ep > 0 {
		return fmt.Sprintf("%s S%02dE%02d", q, season, ep)
	}
	return fmt.Sprintf("%s S%02d", q, season)
}

// handleSearch executes a search and returns Torznab XML results.
func (h *TorznabHandler) handleSearch(w http.ResponseWriter, r *http.Request, runner *scraper.Runner, idx db.Indexer) {
	params := r.URL.Query()
	query := params.Get("q")
	imdbID := params.Get("imdbid")
	t := params.Get("t")
	season, _ := strconv.Atoi(params.Get("season"))
	ep, _ := strconv.Atoi(params.Get("ep"))
	limit, _ := strconv.Atoi(params.Get("limit"))
	offset, _ := strconv.Atoi(params.Get("offset"))

	// Parse category filter
	var cats []int
	if catStr := params.Get("cat"); catStr != "" {
		for _, c := range strings.Split(catStr, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(c)); err == nil {
				cats = append(cats, n)
			}
		}
	}

	// Use IMDB ID as query if provided and no text query
	if query == "" && imdbID != "" {
		query = imdbID
	}

	keywords := composeKeywords(query, season, ep)

	// Check cache. The key is (indexer, composed keywords, categories) —
	// deliberately NOT the t= mode or raw season/ep: composeKeywords
	// already bakes season/ep into the keywords whenever they're
	// distinguishing, and the runner builds identical URLs for tvsearch
	// and plain search with the same keywords. Keying on t or raw
	// season/ep made the standard tvsearch→text fallback (same keywords,
	// no season params) re-scrape instead of hitting the cache — every
	// zero-result query stayed perpetually one scrape behind.
	cacheKey := fmt.Sprintf("%s:%s:%v", idx.ID, keywords, cats)
	if cached, ok := h.engine.Cache().Get(cacheKey); ok {
		h.noteResults(idx.ID, keywords, cached)
		h.writeResults(w, r, idx, cached, limit, offset)
		return
	}

	// Execute via singleflight, detached from any single caller's
	// context. Cloudflare-walled indexers legitimately take 40-90s (a
	// FlareSolverr solve per result page — clearance cookies don't
	// satisfy CF's TLS fingerprinting from a Go client), which impatient
	// clients (Pilot's per-indexer timeout is 25s) never wait out.
	// Detaching lets the scrape complete and land in the cache, so the
	// client's retry is served instantly; singleflight ensures the retry
	// (or a concurrent duplicate) joins the in-flight scrape instead of
	// starting a second one.
	resCh := h.sf.DoChan(cacheKey, func() (interface{}, error) {
		sctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), backgroundSearchBudget)
		defer cancel()
		trace := &scraper.Trace{}
		sctx = scraper.WithTrace(sctx, trace)
		started := time.Now()
		results, err := runner.SearchWithQuery(sctx, scraper.SearchQuery{
			Keywords:   keywords,
			Type:       t,
			Season:     season,
			Ep:         ep,
			IMDBID:     imdbID,
			TMDBID:     params.Get("tmdbid"),
			TVDBID:     params.Get("tvdbid"),
			Categories: cats,
		})
		h.recordSearchOutcome(idx.ID, err)
		h.recordSearchStats(idx.ID, keywords, t, started, results, err, trace)
		if err != nil {
			h.logger.Warn("torznab: search failed",
				"indexer", idx.Name, "query", keywords, "error", err)
			// Failures are NOT cached — a transient error must not be
			// replayed as an empty result set for the cache TTL.
			return nil, err
		}
		h.engine.Cache().Set(cacheKey, results)
		h.logger.Info("torznab: search complete",
			"indexer", idx.Name, "query", keywords, "results", len(results))
		return results, nil
	})

	select {
	case res := <-resCh:
		if res.Err != nil {
			h.xmlError(w, 100, "Search failed: "+res.Err.Error())
			return
		}
		results := res.Val.([]scraper.SearchResult)
		h.noteResults(idx.ID, keywords, results)
		h.writeResults(w, r, idx, results, limit, offset)
	case <-r.Context().Done():
		// Client gave up — the shared scrape continues and will cache.
		h.logger.Info("torznab: client disconnected, scrape continues in background",
			"indexer", idx.Name, "query", keywords)
	}
}

// siteClient fetches .torrent files from indexer sites, through the
// scraper proxy when one is configured.
var siteClient = sitenet.Client(0)

// backgroundSearchBudget bounds a detached scrape after every client has
// disconnected. Generous: a CF-walled multi-page definition needs a
// FlareSolverr solve per page.
const backgroundSearchBudget = 3 * time.Minute

// writeResults encodes search results as Torznab XML.
// Download URLs are rewritten to point at Pulse's download proxy
// so clients don't need CF cookies or tracker sessions.
// limit/offset implement the torznab paging params (0 = no paging).
func (h *TorznabHandler) writeResults(w http.ResponseWriter, r *http.Request, idx db.Indexer, results []scraper.SearchResult, limit, offset int) {
	if offset > 0 {
		if offset >= len(results) {
			results = nil
		} else {
			results = results[offset:]
		}
	}
	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}
	// Build the proxy download base URL.
	// Use the configured external URL when set (Docker), otherwise derive from the request.
	var baseURL string
	if h.externalURL != "" {
		baseURL = h.externalURL
	} else {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		baseURL = fmt.Sprintf("%s://%s", scheme, r.Host)
	}
	proxyBase := fmt.Sprintf("%s/api/v1/torznab/%s/download", baseURL, idx.ID)

	var items []torznab.Item
	for _, res := range results {
		downloadURL := res.Download
		if downloadURL == "" {
			downloadURL = res.MagnetURI
		}

		// Rewrite download URL through the proxy unless it's already a magnet
		enclosureURL := downloadURL
		if downloadURL != "" && !strings.HasPrefix(downloadURL, "magnet:") {
			enclosureURL = proxyBase + "?url=" + url.QueryEscape(downloadURL)
		}

		guid := res.Details
		if guid == "" {
			guid = downloadURL
		}

		items = append(items, torznab.SearchResultToItem(
			res.Title, guid, res.Details, res.Date, res.Size,
			enclosureURL, res.Seeders, res.Leechers,
			res.DownloadVolumeFactor, res.UploadVolumeFactor,
			res.Category, res.InfoHash,
		))
	}

	feed := torznab.NewFeed(idx.Name, items)

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(feed)
}

// torznabError is the Torznab/Newznab error document. Marshalling through
// encoding/xml (rather than fmt-ing a template) guarantees the description is
// attribute-escaped — error messages routinely embed quotes (e.g. an indexer
// name rendered with %q) or "&"/"<", which would otherwise produce invalid
// XML that clients can't parse, masking the real error behind a syntax error.
type torznabError struct {
	XMLName     xml.Name `xml:"error"`
	Code        int      `xml:"code,attr"`
	Description string   `xml:"description,attr"`
}

// xmlError writes a Torznab error response.
func (h *TorznabHandler) xmlError(w http.ResponseWriter, code int, description string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK) // Torznab errors are 200 with error XML
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(torznabError{Code: code, Description: description})
}


// HandleDownload resolves a download URL from a detail page and redirects to it.
// This proxies downloads through Pulse so the client doesn't need CF cookies
// or tracker sessions. The detail page URL is passed via ?url= query param.
//
// GET /api/v1/torznab/{indexer_id}/download?url=https://1337x.to/torrent/12345/...
func (h *TorznabHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	indexerID := chi.URLParam(r, "indexer_id")
	detailURL := r.URL.Query().Get("url")

	if detailURL == "" {
		http.Error(w, "missing url parameter", http.StatusBadRequest)
		return
	}

	// If the URL is already a magnet link, redirect directly
	if strings.HasPrefix(detailURL, "magnet:") {
		http.Redirect(w, r, detailURL, http.StatusFound)
		return
	}

	idx, err := h.q.GetIndexer(r.Context(), indexerID)
	if err != nil {
		http.Error(w, "indexer not found", http.StatusNotFound)
		return
	}

	catalogID := h.engine.ResolveCatalogID(idx.Name, idx.Url)
	if catalogID == "" {
		http.Error(w, "no catalog definition for indexer", http.StatusNotFound)
		return
	}

	runner, err := h.engine.GetRunner(catalogID, idx.Settings)
	if err != nil {
		http.Error(w, "failed to load indexer definition", http.StatusInternalServerError)
		return
	}

	downloadURL, err := runner.ResolveDownload(r.Context(), detailURL)
	if err != nil {
		h.logger.Warn("download: failed to resolve",
			"indexer", idx.Name, "detail_url", detailURL, "error", err)
		http.Error(w, "failed to resolve download: "+err.Error(), http.StatusBadGateway)
		return
	}

	h.logger.Info("download: resolved",
		"indexer", idx.Name,
		"detail_url", detailURL,
		"download_url", downloadURL[:min(80, len(downloadURL))],
	)

	// For magnet links, redirect
	if strings.HasPrefix(downloadURL, "magnet:") {
		http.Redirect(w, r, downloadURL, http.StatusFound)
		return
	}

	// For .torrent URLs, proxy the download so the client gets the file
	// with our CF cookies / sessions intact
	proxyReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, downloadURL, nil)
	if err != nil {
		http.Error(w, "invalid download URL", http.StatusBadGateway)
		return
	}
	proxyReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	proxyResp, err := siteClient.Do(proxyReq)
	if err != nil {
		http.Error(w, "download failed", http.StatusBadGateway)
		return
	}
	defer proxyResp.Body.Close()

	// Pass through content type and body
	w.Header().Set("Content-Type", proxyResp.Header.Get("Content-Type"))
	if cd := proxyResp.Header.Get("Content-Disposition"); cd != "" {
		w.Header().Set("Content-Disposition", cd)
	}
	w.WriteHeader(proxyResp.StatusCode)
	_, _ = io.Copy(w, proxyResp.Body)
}

// TestSearchResult is the JSON response from the test-search endpoint.
type TestSearchResult struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	Duration   string `json:"duration"`
	Results    int    `json:"results"`
	Cloudflare bool   `json:"cloudflare,omitempty"`
}

// HandleTestSearch does a real search through the scraper engine to validate
// that an indexer is functional — not just that its URL responds with HTTP 200.
func (h *TorznabHandler) HandleTestSearch(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	indexerID := chi.URLParam(r, "indexer_id")

	idx, err := h.q.GetIndexer(r.Context(), indexerID)
	if err != nil {
		writeJSON(w, TestSearchResult{Success: false, Message: "Indexer not found", Duration: since(start)})
		return
	}

	catalogID := h.engine.ResolveCatalogID(idx.Name, idx.Url)
	if catalogID == "" {
		writeJSON(w, TestSearchResult{Success: false, Message: fmt.Sprintf("No Prowlarr definition found for %q — this indexer may no longer exist", idx.Name), Duration: since(start)})
		return
	}

	runner, err := h.engine.GetRunner(catalogID, idx.Settings)
	if err != nil {
		writeJSON(w, TestSearchResult{Success: false, Message: "Failed to load definition: " + err.Error(), Duration: since(start)})
		return
	}

	// Do a real search with a generic query to verify the indexer works.
	results, err := runner.Search(r.Context(), "test", nil)
	h.recordSearchOutcome(idx.ID, err)
	if err != nil {
		if scraper.IsCloudflareError(err) {
			writeJSON(w, TestSearchResult{
				Success:     false,
				Message:     "Blocked by Cloudflare — this indexer requires FlareSolverr to bypass browser verification",
				Duration:    since(start),
				Cloudflare:  true,
			})
		} else {
			writeJSON(w, TestSearchResult{Success: false, Message: "Search failed: " + err.Error(), Duration: since(start)})
		}
		return
	}

	if len(results) == 0 {
		writeJSON(w, TestSearchResult{Success: false, Message: "Search returned 0 results — indexer may be down or blocked", Duration: since(start), Results: 0})
		return
	}

	writeJSON(w, TestSearchResult{
		Success:  true,
		Message:  fmt.Sprintf("Working — returned %d results", len(results)),
		Duration: since(start),
		Results:  len(results),
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func since(start time.Time) string {
	return time.Since(start).Truncate(time.Millisecond).String()
}

func newznabBaseID(name string) string {
	switch strings.ToLower(name) {
	case "movies":
		return "2000"
	case "tv":
		return "5000"
	case "audio":
		return "3000"
	case "books":
		return "7000"
	case "xxx":
		return "6000"
	case "pc":
		return "4000"
	default:
		return "8000"
	}
}
