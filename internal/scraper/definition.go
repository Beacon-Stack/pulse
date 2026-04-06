// Package scraper implements a Cardigann-compatible scraping engine that
// executes Prowlarr YAML indexer definitions to scrape tracker websites
// and return structured search results.
package scraper

// Definition is the full parsed representation of a Prowlarr Cardigann YAML file.
type Definition struct {
	ID           string          `yaml:"id"`
	Name         string          `yaml:"name"`
	Description  string          `yaml:"description"`
	Language     string          `yaml:"language"`
	Type         string          `yaml:"type"`     // public, semi-private, private
	Encoding     string          `yaml:"encoding"`
	RequestDelay int             `yaml:"requestDelay"` // milliseconds
	Links        []string        `yaml:"links"`
	LegacyLinks  []string        `yaml:"legacylinks"`
	Caps         CapsBlock       `yaml:"caps"`
	Settings     []SettingField  `yaml:"settings"`
	Login        *LoginBlock     `yaml:"login,omitempty"`
	Search       SearchBlock     `yaml:"search"`
	Download     DownloadBlock   `yaml:"download"`
}

// CapsBlock describes what the indexer supports.
type CapsBlock struct {
	CategoryMappings []CategoryMapping      `yaml:"categorymappings"`
	Modes            map[string][]string    `yaml:"modes"`
	AllowRawSearch   bool                   `yaml:"allowrawsearch"`
}

// CategoryMapping maps a site-specific category ID to a Newznab category.
type CategoryMapping struct {
	ID   string `yaml:"id"`
	Cat  string `yaml:"cat"`
	Desc string `yaml:"desc"`
}

// SettingField is a user-configurable field in the YAML definition.
// Uses interface{} for Default and Options to handle varied YAML types.
type SettingField struct {
	Name    string      `yaml:"name"`
	Type    string      `yaml:"type"`
	Label   string      `yaml:"label"`
	Default interface{} `yaml:"default"`
	Options interface{} `yaml:"options"` // map[string]string or []map[string]string
}

// LoginBlock describes how to authenticate with a private tracker.
type LoginBlock struct {
	Path    string            `yaml:"path"`
	Method  string            `yaml:"method"` // post, get, form, cookie
	Inputs  map[string]string `yaml:"inputs"`
	Error   []SelectorBlock   `yaml:"error"`
	Test    *TestBlock        `yaml:"test"`
}

// TestBlock verifies a login succeeded.
type TestBlock struct {
	Path     string `yaml:"path"`
	Selector string `yaml:"selector"`
}

// SearchBlock defines how to search the tracker.
type SearchBlock struct {
	Paths           []SearchPath          `yaml:"paths"`
	Inputs          map[string]string     `yaml:"inputs"`
	KeywordsFilters []FilterDef           `yaml:"keywordsfilters"`
	Rows            RowsBlock             `yaml:"rows"`
	Fields          map[string]FieldBlock `yaml:"fields"`
}

// SearchPath is a single search URL pattern.
type SearchPath struct {
	Path     string            `yaml:"path"`
	Method   string            `yaml:"method"`
	Inputs   map[string]string `yaml:"inputs"`
	Response *ResponseConfig   `yaml:"response,omitempty"`
}

// ResponseConfig specifies the response format.
type ResponseConfig struct {
	Type             string `yaml:"type"` // html (default), json, xml
	NoResultsMessage string `yaml:"noResultsMessage"`
}

// RowsBlock defines how to find result rows in the response.
type RowsBlock struct {
	Selector  string `yaml:"selector"`
	After     int    `yaml:"after"`
	Remove    string `yaml:"remove"`
	Multiple  bool   `yaml:"multiple"`
	Count     *SelectorBlock `yaml:"count,omitempty"`
}

// FieldBlock defines how to extract a single field from a result row.
type FieldBlock struct {
	SelectorBlock `yaml:",inline"`
	Optional      bool `yaml:"optional"`
}

// SelectorBlock is the core unit for extracting data from HTML/JSON.
type SelectorBlock struct {
	Selector  string            `yaml:"selector"`
	Text      string            `yaml:"text"`
	Attribute string            `yaml:"attribute"`
	Remove    string            `yaml:"remove"`
	Filters   []FilterDef       `yaml:"filters"`
	Case      map[string]string `yaml:"case"`
}

// FilterDef describes a single filter in the pipeline.
type FilterDef struct {
	Name string      `yaml:"name"`
	Args interface{} `yaml:"args"` // string, []string, or []interface{}
}

// DownloadBlock defines how to extract download links.
type DownloadBlock struct {
	Selectors []SelectorBlock `yaml:"selectors"`
	Method    string          `yaml:"method"`
}

// SearchResult is a single result from a search execution.
type SearchResult struct {
	Title                string
	Details              string  // URL to the detail page
	Download             string  // .torrent URL or magnet URI
	MagnetURI            string
	InfoHash             string
	Size                 int64
	Seeders              int
	Leechers             int
	Grabs                int
	Date                 string  // RFC3339 or raw
	Category             string  // Newznab category ID
	DownloadVolumeFactor float64
	UploadVolumeFactor   float64
	Description          string
	IMDBID               string
	Poster               string
}
