package indexer

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// prowlarrSnapshot is a build-time snapshot of Prowlarr's definitions/v11
// directory. It guarantees the scraper engine has working definitions from
// first boot even when GitHub is unreachable (the live fetch then only
// freshens them). Regenerate with `make prowlarr-snapshot`.
//
//go:embed prowlarr_snapshot.zip
var prowlarrSnapshot []byte

// SetCachePath enables persisting each successfully fetched definitions
// zipball to path, and makes LoadLocal prefer that file over the embedded
// snapshot. Call before LoadLocal / StartRefreshLoop.
func (pc *ProwlarrCatalog) SetCachePath(path string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.cachePath = path
}

// SetUpdateFunc registers fn to be called with a copy of the raw YAML
// definition map whenever definitions are (re)loaded from any source —
// local load at startup and every successful GitHub refresh. Call before
// LoadLocal / StartRefreshLoop.
func (pc *ProwlarrCatalog) SetUpdateFunc(fn func(map[string][]byte)) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.onUpdate = fn
}

// LoadLocal populates the catalog without touching the network, preferring
// the freshest source: the on-disk cache written after the last successful
// GitHub fetch, then the snapshot embedded at build time. It returns which
// source was used ("disk-cache" or "embedded").
func (pc *ProwlarrCatalog) LoadLocal() (string, error) {
	pc.mu.RLock()
	cachePath := pc.cachePath
	pc.mu.RUnlock()

	if cachePath != "" {
		if zipData, err := os.ReadFile(cachePath); err == nil {
			entries, rawYAML, errCount, perr := parseDefinitionsZip(zipData)
			if perr == nil && len(rawYAML) > 0 {
				// Best-effort fetch time: the cache file's mtime is when the
				// zipball was last downloaded.
				var fetched time.Time
				if fi, statErr := os.Stat(cachePath); statErr == nil {
					fetched = fi.ModTime().UTC()
				}
				total := pc.apply(entries, rawYAML, fetched)
				pc.logger.Info("prowlarr: loaded cached definitions",
					"path", cachePath, "indexers", total, "parse_errors", errCount)
				return "disk-cache", nil
			}
			pc.logger.Warn("prowlarr: cached definitions unreadable, falling back to embedded snapshot",
				"path", cachePath, "error", perr)
		}
	}

	entries, rawYAML, errCount, err := parseDefinitionsZip(prowlarrSnapshot)
	if err != nil {
		return "", fmt.Errorf("parsing embedded snapshot: %w", err)
	}
	if len(rawYAML) == 0 {
		return "", fmt.Errorf("embedded snapshot contains no definitions")
	}
	total := pc.apply(entries, rawYAML, time.Time{})
	pc.logger.Info("prowlarr: loaded embedded snapshot definitions",
		"indexers", total, "parse_errors", errCount)
	return "embedded", nil
}

// saveCache atomically writes the fetched zipball to the cache path so the
// next boot can load it without GitHub. Best-effort: failures are logged,
// never fatal — the in-memory catalog is already updated.
func (pc *ProwlarrCatalog) saveCache(zipData []byte) {
	pc.mu.RLock()
	cachePath := pc.cachePath
	pc.mu.RUnlock()
	if cachePath == "" {
		return
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		pc.logger.Warn("prowlarr: cannot create definitions cache dir", "path", cachePath, "error", err)
		return
	}
	tmp := cachePath + ".tmp"
	if err := os.WriteFile(tmp, zipData, 0o644); err != nil {
		pc.logger.Warn("prowlarr: cannot write definitions cache", "path", tmp, "error", err)
		return
	}
	if err := os.Rename(tmp, cachePath); err != nil {
		pc.logger.Warn("prowlarr: cannot move definitions cache into place", "path", cachePath, "error", err)
		return
	}
	pc.logger.Info("prowlarr: cached definitions to disk", "path", cachePath, "bytes", len(zipData))
}
