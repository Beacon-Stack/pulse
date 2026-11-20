package scraper

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// Engine is the top-level scraping coordinator. It manages definitions,
// runners, and the result cache.
type Engine struct {
	rawStore     map[string][]byte  // catalog_id → raw YAML bytes
	nameToID     map[string]string  // normalized name → catalog_id
	urlToID      map[string]string  // domain → catalog_id
	mu           sync.RWMutex
	runners      sync.Map           // catalog_id + "\x00" + settings JSON → *Runner
	cache        *ResultCache
	flaresolverr *FlareSolverr
	logger       *slog.Logger
}

// NewEngine creates a new scraping engine.
// flaresolverr can be nil if not configured.
func NewEngine(logger *slog.Logger, flaresolverr *FlareSolverr) *Engine {
	return &Engine{
		rawStore:     make(map[string][]byte),
		nameToID:     make(map[string]string),
		urlToID:      make(map[string]string),
		cache:        NewResultCache(0),
		flaresolverr: flaresolverr,
		logger:       logger,
	}
}

// LoadDefinitions stores raw YAML bytes for all catalog entries.
// Called after the Prowlarr zip is fetched.
func (e *Engine) LoadDefinitions(defs map[string][]byte) {
	nameMap := make(map[string]string, len(defs))
	urlMap := make(map[string]string, len(defs))

	for id, raw := range defs {
		def, err := ParseDefinition(raw)
		if err != nil {
			continue
		}
		// Map normalized name → catalog ID
		norm := normalizeName(def.Name)
		nameMap[norm] = id
		// Also map the ID itself as a name
		nameMap[normalizeName(id)] = id

		// Map domains → catalog ID
		for _, u := range def.Links {
			domain := extractDomain(u)
			if domain != "" {
				urlMap[domain] = id
			}
		}
	}

	e.mu.Lock()
	e.rawStore = defs
	e.nameToID = nameMap
	e.urlToID = urlMap
	e.runners = sync.Map{}
	e.mu.Unlock()

	e.logger.Info("scraper: loaded definition YAML",
		"count", len(defs),
		"name_mappings", len(nameMap),
		"url_mappings", len(urlMap),
	)
}

// HasDefinition checks if a definition exists for the given catalog ID.
func (e *Engine) HasDefinition(catalogID string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, ok := e.rawStore[catalogID]
	return ok
}

// GetRunner returns a cached or new Runner for the given catalog ID and settings.
func (e *Engine) GetRunner(catalogID string, settingsJSON string) (*Runner, error) {
	// The cache key must include the settings: keying on catalogID alone
	// meant a settings change in Pulse (e.g. fixing Nyaa's cat-id) never
	// reached the scraper until a process restart — the runner kept its
	// stale merged settings forever.
	cacheKey := catalogID + "\x00" + settingsJSON
	if r, ok := e.runners.Load(cacheKey); ok {
		return r.(*Runner), nil
	}

	// Parse definition
	e.mu.RLock()
	raw, ok := e.rawStore[catalogID]
	e.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no definition found for catalog ID %q", catalogID)
	}

	def, err := ParseDefinition(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing definition for %q: %w", catalogID, err)
	}

	// Parse user settings
	settings := make(map[string]string)
	if settingsJSON != "" && settingsJSON != "{}" {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(settingsJSON), &raw); err == nil {
			for k, v := range raw {
				settings[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	runner := NewRunner(def, settings, e.flaresolverr, e.logger)

	// Cache it
	e.runners.Store(cacheKey, runner)

	return runner, nil
}

// Cache returns the result cache.
func (e *Engine) Cache() *ResultCache {
	return e.cache
}

// ClearScraperState drops all IP-bound scraper state: cached FlareSolverr
// Cloudflare sessions and cached search results. Called after a VPN IP
// rotation — both are tied to the egress IP that produced them.
func (e *Engine) ClearScraperState() {
	e.flaresolverr.ClearSessions() // nil-safe
	e.cache.Clear()
}

// ChallengeTroubles lists sites whose Cloudflare challenges keep failing
// on the current egress IP. Empty without FlareSolverr.
func (e *Engine) ChallengeTroubles() []SiteTrouble {
	return e.flaresolverr.ChallengeTroubles() // nil-safe
}

// ResolveCatalogID finds the Prowlarr catalog ID for an indexer by trying:
// 1. Exact catalog ID match
// 2. Normalized name match (e.g., "The Pirate Bay" → "thepiratebay")
// 3. URL domain match (e.g., "https://1337x.to" → looks up "1337x.to")
func (e *Engine) ResolveCatalogID(name, url string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Exact ID
	if _, ok := e.rawStore[name]; ok {
		return name
	}

	// 2. Normalized name
	norm := normalizeName(name)
	if id, ok := e.nameToID[norm]; ok {
		return id
	}

	// 3. URL domain
	domain := extractDomain(url)
	if domain != "" {
		if id, ok := e.urlToID[domain]; ok {
			return id
		}
	}

	return ""
}

// DefinitionCount returns how many definitions are loaded.
func (e *Engine) DefinitionCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.rawStore)
}

// normalizeName strips spaces, dots, dashes and lowercases for fuzzy matching.
func normalizeName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	return s
}

// extractDomain pulls the domain from a URL (e.g., "https://1337x.to/" → "1337x.to").
func extractDomain(rawURL string) string {
	rawURL = strings.TrimRight(rawURL, "/")
	// Strip protocol
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(rawURL, prefix) {
			rawURL = rawURL[len(prefix):]
			break
		}
	}
	// Strip path
	if idx := strings.Index(rawURL, "/"); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	return strings.ToLower(rawURL)
}
