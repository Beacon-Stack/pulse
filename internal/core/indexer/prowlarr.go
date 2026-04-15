package indexer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// GitHub API endpoint that returns the full repo tree in one call.
	prowlarrTreeURL = "https://api.github.com/repos/Prowlarr/Indexers/git/trees/master?recursive=1"
	// Raw file URL template.
	prowlarrRawURL = "https://raw.githubusercontent.com/Prowlarr/Indexers/master/"
	// Only import v11 definitions (current stable).
	v11Prefix = "definitions/v11/"
	v11Suffix = ".yml"

	fetchTimeout   = 30 * time.Second
	refreshDefault = 24 * time.Hour
)

// prowlarrDef is the subset of a Cardigann YAML definition we parse.
// Settings use interface{} because Prowlarr's YAML has varied option formats
// that cause strict struct unmarshaling to fail.
type prowlarrDef struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Language    string   `yaml:"language"`
	Type        string   `yaml:"type"`     // public, semi-private, private
	Links       []string `yaml:"links"`
	LegacyLinks []string `yaml:"legacylinks"`
	Caps        struct {
		CategoryMappings []struct {
			Cat string `yaml:"cat"`
		} `yaml:"categorymappings"`
	} `yaml:"caps"`
	Settings []map[string]interface{} `yaml:"settings"`
}

// ProwlarrCatalog manages a catalog sourced from Prowlarr's GitHub repo.
type ProwlarrCatalog struct {
	mu       sync.RWMutex
	entries  []CatalogEntry
	fetched  time.Time
	client   *http.Client
	logger   *slog.Logger
	fallback []CatalogEntry // static fallback if fetch fails
	rawYAML  map[string][]byte // catalog_id → raw YAML bytes for the scraper engine
}

// NewProwlarrCatalog creates a catalog that pulls from Prowlarr's definitions.
// It initializes with the static fallback and fetches live data asynchronously.
func NewProwlarrCatalog(logger *slog.Logger) *ProwlarrCatalog {
	pc := &ProwlarrCatalog{
		entries:  builtinCatalog,
		client:   &http.Client{Timeout: fetchTimeout},
		logger:   logger,
		fallback: builtinCatalog,
		rawYAML:  make(map[string][]byte),
	}
	return pc
}

// RawYAML returns the raw YAML bytes for a catalog entry, used by the scraper engine.
func (pc *ProwlarrCatalog) RawYAML(catalogID string) ([]byte, bool) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	raw, ok := pc.rawYAML[catalogID]
	return raw, ok
}

// AllRawYAML returns all raw YAML bytes, keyed by catalog ID.
func (pc *ProwlarrCatalog) AllRawYAML() map[string][]byte {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	out := make(map[string][]byte, len(pc.rawYAML))
	for k, v := range pc.rawYAML {
		out[k] = v
	}
	return out
}

// Entries returns the current catalog entries.
func (pc *ProwlarrCatalog) Entries() []CatalogEntry {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.entries
}

// LastFetched returns when the catalog was last refreshed from Prowlarr.
func (pc *ProwlarrCatalog) LastFetched() time.Time {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.fetched
}

// Refresh fetches the latest definitions from Prowlarr's GitHub repo.
// On failure it keeps the existing catalog (either previous fetch or static fallback).
func (pc *ProwlarrCatalog) Refresh(ctx context.Context) error {
	pc.logger.Info("prowlarr: fetching indexer definitions from GitHub...")

	// Step 1: Get the file list from the repo tree.
	files, err := pc.listV11Files(ctx)
	if err != nil {
		return fmt.Errorf("listing v11 files: %w", err)
	}

	pc.logger.Info("prowlarr: found definition files", "count", len(files))

	// Fetch all definitions via the zipball to avoid per-file rate limits.
	pc.logger.Info("prowlarr: downloading zipball...")
	entries, errCount, err := pc.fetchAllViaZip(ctx, files)
	if err != nil {
		pc.logger.Warn("prowlarr: zipball fetch failed, trying individual files", "error", err)
		entries, errCount = pc.fetchIndividual(ctx, files)
	} else {
		pc.logger.Info("prowlarr: zipball parsed", "entries", len(entries), "errors", errCount)
	}

	if errCount > 0 {
		pc.logger.Warn("prowlarr: some definitions failed to parse", "failed", errCount, "succeeded", len(entries))
	}

	// Prowlarr's YAML definitions only cover torrent indexers. Usenet indexers
	// are native C# implementations without YAML files. Merge in our static
	// usenet + generic entries so the catalog is complete.
	for _, static := range builtinCatalog {
		if static.Protocol == "usenet" || strings.HasPrefix(static.ID, "generic-") {
			entries = append(entries, static)
		}
	}

	if len(entries) == 0 {
		return fmt.Errorf("no definitions parsed successfully")
	}

	pc.mu.Lock()
	pc.entries = entries
	pc.fetched = time.Now().UTC()
	pc.mu.Unlock()

	pc.logger.Info("prowlarr: catalog updated", "indexers", len(entries))
	return nil
}

// StartRefreshLoop refreshes the catalog immediately, then periodically.
func (pc *ProwlarrCatalog) StartRefreshLoop(ctx context.Context, interval time.Duration) {
	if interval == 0 {
		interval = refreshDefault
	}

	if err := pc.Refresh(ctx); err != nil {
		pc.logger.Warn("prowlarr: initial catalog refresh failed, using static fallback", "error", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := pc.Refresh(ctx); err != nil {
				pc.logger.Warn("prowlarr: periodic catalog refresh failed", "error", err)
			}
		}
	}
}

// listV11Files returns the paths of all v11 YAML definition files.
func (pc *ProwlarrCatalog) listV11Files(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, prowlarrTreeURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := pc.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, fmt.Errorf("decoding tree: %w", err)
	}

	var files []string
	for _, item := range tree.Tree {
		if item.Type == "blob" && strings.HasPrefix(item.Path, v11Prefix) && strings.HasSuffix(item.Path, v11Suffix) {
			files = append(files, item.Path)
		}
	}
	return files, nil
}

// fetchAllViaZip downloads the repo zipball and extracts all v11 YAML definitions.
// This is a single HTTP request instead of 548 individual ones.
func (pc *ProwlarrCatalog) fetchAllViaZip(ctx context.Context, _ []string) (entries []CatalogEntry, errCount int64, err error) {
	// Use the codeload URL directly to avoid the API 302 redirect.
	zipURL := "https://codeload.github.com/Prowlarr/Indexers/legacy.zip/refs/heads/master"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, zipURL, nil)
	if err != nil {
		return nil, 0, err
	}

	// Use a longer timeout for the full zip download.
	zipClient := &http.Client{Timeout: 120 * time.Second}
	resp, err := zipClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, 0, fmt.Errorf("zipball returned HTTP %d", resp.StatusCode)
	}

	// Read the entire zip into memory (it's ~2-3 MB).
	zipData, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if err != nil {
		return nil, 0, fmt.Errorf("reading zipball: %w", err)
	}

	// Parse the zip.
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, 0, fmt.Errorf("opening zip: %w", err)
	}

	rawYAML := make(map[string][]byte)

	for _, f := range zipReader.File {
		// Zip paths look like: Prowlarr-Indexers-abc123/definitions/v11/1337x.yml
		name := f.Name
		idx := strings.Index(name, "definitions/v11/")
		if idx < 0 || !strings.HasSuffix(name, ".yml") || f.FileInfo().IsDir() {
			continue
		}

		rc, openErr := f.Open()
		if openErr != nil {
			errCount++
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(rc, 256*1024))
		rc.Close()
		if readErr != nil {
			errCount++
			continue
		}

		var def prowlarrDef
		if yamlErr := yaml.Unmarshal(body, &def); yamlErr != nil {
			errCount++
			continue
		}

		entry := defToEntry(def)
		if entry.Name != "" && len(entry.URLs) > 0 {
			entries = append(entries, entry)
			// Store the raw YAML for the scraper engine
			rawYAML[def.ID] = body
		}
	}

	// Store raw YAML on the catalog for the scraper engine.
	pc.mu.Lock()
	pc.rawYAML = rawYAML
	pc.mu.Unlock()

	return entries, errCount, nil
}

// fetchIndividual fetches definitions one at a time (fallback if zipball fails).
func (pc *ProwlarrCatalog) fetchIndividual(ctx context.Context, files []string) ([]CatalogEntry, int64) {
	type result struct {
		entry CatalogEntry
		ok    bool
	}

	sem := make(chan struct{}, 5)
	results := make(chan result, len(files))
	var errCount int64

	var wg sync.WaitGroup
	for _, path := range files {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			entry, err := pc.fetchAndParse(ctx, p)
			if err != nil {
				atomic.AddInt64(&errCount, 1)
				return
			}
			results <- result{entry: entry, ok: true}
		}(path)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var entries []CatalogEntry
	for r := range results {
		if r.ok {
			entries = append(entries, r.entry)
		}
	}

	return entries, errCount
}

// fetchAndParse fetches a single YAML file and converts it to a CatalogEntry.
func (pc *ProwlarrCatalog) fetchAndParse(ctx context.Context, path string) (CatalogEntry, error) {
	url := prowlarrRawURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return CatalogEntry{}, err
	}

	resp, err := pc.client.Do(req)
	if err != nil {
		return CatalogEntry{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return CatalogEntry{}, fmt.Errorf("HTTP %d for %s", resp.StatusCode, path)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return CatalogEntry{}, err
	}

	var def prowlarrDef
	if err := yaml.Unmarshal(body, &def); err != nil {
		return CatalogEntry{}, fmt.Errorf("parsing YAML: %w", err)
	}

	return defToEntry(def), nil
}

// defToEntry converts a Prowlarr YAML definition to our CatalogEntry.
func defToEntry(def prowlarrDef) CatalogEntry {
	// Determine protocol from the definition type and categories.
	protocol := "torrent"
	for _, cm := range def.Caps.CategoryMappings {
		cat := strings.ToLower(cm.Cat)
		if strings.Contains(cat, "newznab") || strings.Contains(cat, "nzb") {
			protocol = "usenet"
			break
		}
	}

	// Extract unique normalized categories.
	catSet := map[string]bool{}
	for _, cm := range def.Caps.CategoryMappings {
		cat := normalizeCategory(cm.Cat)
		if cat != "" {
			catSet[cat] = true
		}
	}
	var categories []string
	for cat := range catSet {
		categories = append(categories, cat)
	}
	if len(categories) == 0 {
		categories = []string{"Other"}
	}

	// Convert settings from map[string]interface{} to Field structs.
	var settings []Field
	for _, s := range def.Settings {
		name := strVal(s, "name")
		typ := strVal(s, "type")
		label := strVal(s, "label")

		if name == "" {
			continue
		}

		required := false
		if def.Type == "private" || def.Type == "semi-private" {
			switch name {
			case "username", "password", "apikey", "api_key", "passkey", "cookie":
				required = true
			}
		}

		f := Field{
			Name:        name,
			Type:        mapFieldType(typ),
			Label:       label,
			HelpText:    strVal(s, "helptext"),
			Required:    required,
			Default:     strVal(s, "default"),
			Placeholder: strVal(s, "placeholder"),
		}

		// Parse options if present.
		if opts, ok := s["options"]; ok {
			if optSlice, ok := opts.([]interface{}); ok {
				for _, o := range optSlice {
					if om, ok := o.(map[string]interface{}); ok {
						f.Options = append(f.Options, FieldOption{
							Name:  strValMap(om, "name"),
							Value: strValMap(om, "value"),
						})
					}
				}
			}
		}

		settings = append(settings, f)
	}

	// Clean up URLs — remove trailing slashes for consistency.
	var urls []string
	for _, u := range def.Links {
		urls = append(urls, strings.TrimRight(u, "/"))
	}

	privacy := def.Type
	if privacy == "" {
		privacy = "public"
	}

	return CatalogEntry{
		ID:          def.ID,
		Name:        def.Name,
		Description: def.Description,
		Language:    def.Language,
		Protocol:    protocol,
		Privacy:     privacy,
		Categories:  categories,
		URLs:        urls,
		Settings:    settings,
	}
}

// strVal extracts a string value from a map[string]interface{}.
func strVal(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// strValMap is the same as strVal but for map[string]interface{}.
func strValMap(m map[string]interface{}, key string) string {
	return strVal(m, key)
}

// normalizeCategory maps Prowlarr's Newznab-style category strings to our
// simplified category names.
func normalizeCategory(cat string) string {
	c := strings.ToLower(cat)
	switch {
	case strings.HasPrefix(c, "movies"), strings.HasPrefix(c, "movie"):
		return "Movies"
	case strings.HasPrefix(c, "tv"):
		return "TV"
	case strings.HasPrefix(c, "audio"), strings.HasPrefix(c, "music"):
		return "Audio"
	case strings.HasPrefix(c, "books"), strings.HasPrefix(c, "book"), strings.HasPrefix(c, "ebook"):
		return "Books"
	case strings.HasPrefix(c, "xxx"), strings.HasPrefix(c, "adult"):
		return "XXX"
	case strings.HasPrefix(c, "pc"), strings.HasPrefix(c, "console"), strings.HasPrefix(c, "other"):
		return "Other"
	default:
		return "Other"
	}
}

// mapFieldType normalizes Prowlarr field types to our simplified set.
func mapFieldType(t string) string {
	switch strings.ToLower(t) {
	case "text", "":
		return "text"
	case "password":
		return "password"
	case "checkbox":
		return "checkbox"
	case "select", "multi-select":
		return "select"
	default:
		if strings.HasPrefix(strings.ToLower(t), "info") {
			return "info"
		}
		return "text"
	}
}
