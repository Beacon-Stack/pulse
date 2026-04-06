package v1

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	dbsqlite "github.com/arrsenal/configurarr/internal/db/generated/sqlite"
	"github.com/arrsenal/configurarr/internal/scraper"
	"github.com/arrsenal/configurarr/internal/torznab"
)

// TorznabHandler handles Torznab proxy requests.
type TorznabHandler struct {
	engine *scraper.Engine
	q      dbsqlite.Querier
	logger *slog.Logger
}

// NewTorznabHandler creates a new handler.
func NewTorznabHandler(engine *scraper.Engine, q dbsqlite.Querier, logger *slog.Logger) *TorznabHandler {
	return &TorznabHandler{engine: engine, q: q, logger: logger}
}

// RegisterTorznabRoutes registers the Torznab proxy endpoint on the chi router.
// This is registered directly on chi (not Huma) because Torznab uses XML, not JSON.
func RegisterTorznabRoutes(r chi.Router, h *TorznabHandler) {
	r.Get("/api/v1/torznab/{indexer_id}/api", h.Handle)
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
func (h *TorznabHandler) handleCaps(w http.ResponseWriter, runner *scraper.Runner, idx dbsqlite.Indexer) {
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
	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(caps)
}

// handleSearch executes a search and returns Torznab XML results.
func (h *TorznabHandler) handleSearch(w http.ResponseWriter, r *http.Request, runner *scraper.Runner, idx dbsqlite.Indexer) {
	query := r.URL.Query().Get("q")
	imdbID := r.URL.Query().Get("imdbid")

	// Parse category filter
	var cats []int
	if catStr := r.URL.Query().Get("cat"); catStr != "" {
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

	// Check cache
	cacheKey := fmt.Sprintf("%s:%s:%v", idx.ID, query, cats)
	if cached, ok := h.engine.Cache().Get(cacheKey); ok {
		h.writeResults(w, idx.Name, cached)
		return
	}

	// Execute search
	results, err := runner.Search(context.Background(), query, cats)
	if err != nil {
		h.logger.Warn("torznab: search failed",
			"indexer", idx.Name, "query", query, "error", err)
		h.xmlError(w, 100, "Search failed: "+err.Error())
		return
	}

	// Cache results
	h.engine.Cache().Set(cacheKey, results)

	h.logger.Info("torznab: search complete",
		"indexer", idx.Name, "query", query, "results", len(results))

	h.writeResults(w, idx.Name, results)
}

// writeResults encodes search results as Torznab XML.
func (h *TorznabHandler) writeResults(w http.ResponseWriter, indexerName string, results []scraper.SearchResult) {
	var items []torznab.Item
	for _, r := range results {
		downloadURL := r.Download
		if downloadURL == "" {
			downloadURL = r.MagnetURI
		}

		guid := r.Details
		if guid == "" {
			guid = downloadURL
		}

		items = append(items, torznab.SearchResultToItem(
			r.Title, guid, r.Details, r.Date, r.Size,
			downloadURL, r.Seeders, r.Leechers,
			r.DownloadVolumeFactor, r.UploadVolumeFactor,
			r.Category,
		))
	}

	feed := torznab.NewFeed(indexerName, items)

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(feed)
}

// xmlError writes a Torznab error response.
func (h *TorznabHandler) xmlError(w http.ResponseWriter, code int, description string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK) // Torznab errors are 200 with error XML
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<error code="%d" description="%s"/>`, code, description)
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
	json.NewEncoder(w).Encode(v)
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
