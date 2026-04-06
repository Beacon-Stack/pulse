package indexer

import "sync"

// CatalogEntry describes an available indexer that users can browse and add.
// This is the "template" — not yet configured or saved.
type CatalogEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Language    string   `json:"language"`
	Protocol    string   `json:"protocol"`    // torrent, usenet
	Privacy     string   `json:"privacy"`     // public, semi-private, private
	Categories  []string `json:"categories"`  // Movies, TV, Audio, Books, XXX, Other
	URLs        []string `json:"urls"`
	Settings    []Field  `json:"settings"`    // config fields required to add this indexer
}

// Field describes a single user-configurable setting for an indexer.
type Field struct {
	Name        string        `json:"name"`
	Type        string        `json:"type"`        // text, password, checkbox, select, info
	Label       string        `json:"label"`
	HelpText    string        `json:"help_text,omitempty"`
	Required    bool          `json:"required"`
	Default     string        `json:"default,omitempty"`
	Placeholder string        `json:"placeholder,omitempty"`
	Options     []FieldOption `json:"options,omitempty"` // for select type
}

// FieldOption is a name/value pair for select fields.
type FieldOption struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CatalogFilter holds the query parameters for browsing the catalog.
type CatalogFilter struct {
	Query    string `json:"query,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Privacy  string `json:"privacy,omitempty"`
	Category string `json:"category,omitempty"`
	Language string `json:"language,omitempty"`
}

// ── Global catalog reference ─────────────────────────────────────────────────
// Set once at startup via SetCatalogSource. Defaults to the static builtin.

var (
	catalogMu     sync.RWMutex
	catalogSource func() []CatalogEntry = func() []CatalogEntry { return builtinCatalog }
)

// SetCatalogSource replaces the catalog provider function. Call this once at
// startup after creating a ProwlarrCatalog.
func SetCatalogSource(fn func() []CatalogEntry) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	catalogSource = fn
}

// Catalog returns the current catalog entries (from Prowlarr or static fallback).
func Catalog() []CatalogEntry {
	catalogMu.RLock()
	fn := catalogSource
	catalogMu.RUnlock()
	return fn()
}

// FilterCatalog returns catalog entries matching the given filter.
func FilterCatalog(filter CatalogFilter) []CatalogEntry {
	all := Catalog()
	if filter.Query == "" && filter.Protocol == "" && filter.Privacy == "" && filter.Category == "" && filter.Language == "" {
		return all
	}

	var out []CatalogEntry
	for _, e := range all {
		if !matchEntry(e, filter) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// CatalogLanguages returns the distinct languages present in the catalog.
func CatalogLanguages() []string {
	all := Catalog()
	seen := map[string]bool{}
	var out []string
	for _, e := range all {
		if !seen[e.Language] {
			seen[e.Language] = true
			out = append(out, e.Language)
		}
	}
	return out
}

func matchEntry(e CatalogEntry, f CatalogFilter) bool {
	if f.Protocol != "" && e.Protocol != f.Protocol {
		return false
	}
	if f.Privacy != "" && e.Privacy != f.Privacy {
		return false
	}
	if f.Category != "" && !containsStr(e.Categories, f.Category) {
		return false
	}
	if f.Language != "" && !eqFold(e.Language, f.Language) {
		return false
	}
	if f.Query != "" && !fuzzyMatch(e, f.Query) {
		return false
	}
	return true
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if eqFold(v, s) {
			return true
		}
	}
	return false
}

func fuzzyMatch(e CatalogEntry, q string) bool {
	q = toLower(q)
	if containsLower(e.Name, q) {
		return true
	}
	if containsLower(e.Description, q) {
		return true
	}
	for _, cat := range e.Categories {
		if containsLower(cat, q) {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range len(s) {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func containsLower(haystack, needle string) bool {
	h := toLower(haystack)
	n := needle // already lowered
	if len(n) > len(h) {
		return false
	}
	for i := 0; i <= len(h)-len(n); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}

func eqFold(a, b string) bool {
	return toLower(a) == toLower(b)
}
