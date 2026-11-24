package scraper

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// CloudflareError indicates the request was blocked by Cloudflare protection.
type CloudflareError struct {
	StatusCode int
}

func (e *CloudflareError) Error() string {
	return fmt.Sprintf("Cloudflare protection detected (HTTP %d) — requires FlareSolverr", e.StatusCode)
}

// IsCloudflareError checks if an error is a Cloudflare block.
func IsCloudflareError(err error) bool {
	var cfErr *CloudflareError
	return errors.As(err, &cfErr)
}

// HTTPStatusError is a non-OK HTTP status from an indexer site.
type HTTPStatusError struct {
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

// Runner executes searches against a single indexer using its YAML definition.
type Runner struct {
	def          *Definition
	client       *RateLimitedClient
	flaresolverr *FlareSolverr
	settings     map[string]string // user-provided config values
	baseURL      string            // primary site URL (the definition's first link)
	// links are the site's addresses in definition order (mirrors after
	// the first); active indexes the one that last worked.
	links  []string
	active atomic.Int32
	logger *slog.Logger
}

// NewRunner creates a runner for the given definition.
func NewRunner(def *Definition, settings map[string]string, flaresolverr *FlareSolverr, logger *slog.Logger) *Runner {
	delay := time.Duration(def.RequestDelay) * time.Millisecond
	baseURL := ""
	if len(def.Links) > 0 {
		baseURL = strings.TrimRight(def.Links[0], "/")
	}
	var links []string
	for _, l := range def.Links {
		if l = strings.TrimRight(strings.TrimSpace(l), "/"); l != "" {
			links = append(links, l)
		}
	}

	// Merge settings with defaults from the definition.
	//
	// Boolean defaults need special handling: Cardigann YAML uses
	// `{{ if .Config.foo }}` expecting a bool false to skip the branch.
	// Storing it as the string "false" makes Go's `if` treat it as truthy
	// (any non-empty string is truthy), which selects the wrong template
	// branch — this is what produced "<no value>" titles for Nyaa.si
	// (the upstream YAML's title field has
	// `{{ if .Config.sonarr_compatibility }}…{{ end }}` and the false
	// default fell into the truthy branch). Map booleans to ""/`"true"`
	// so the string is empty (falsy) when the bool is false.
	//
	// User-supplied values get the same coercion BUT only for settings
	// declared as `type: checkbox` — Cardigann YAML also uses literal
	// "0" as a valid filter/category ID, which we must not silently
	// convert to "" (truthy, but resolves to wrong category).
	checkboxFields := make(map[string]bool)
	for _, s := range def.Settings {
		if strings.EqualFold(s.Type, "checkbox") {
			checkboxFields[s.Name] = true
		}
	}
	merged := make(map[string]string)
	for _, s := range def.Settings {
		if s.Default != nil {
			merged[s.Name] = configValueAsTemplateString(s.Default)
		}
	}
	for k, v := range settings {
		if checkboxFields[k] {
			merged[k] = normalizeBoolString(v)
		} else {
			merged[k] = v
		}
	}
	// Inject sitelink
	merged["sitelink"] = baseURL + "/"

	return &Runner{
		def:          def,
		client:       NewRateLimitedClient(delay),
		flaresolverr: flaresolverr,
		settings:     merged,
		baseURL:      baseURL,
		links:        links,
		logger:       logger,
	}
}

// Definition returns the underlying definition.
func (r *Runner) Definition() *Definition {
	return r.def
}

// configValueAsTemplateString coerces a YAML setting default to the string
// form Go templates can branch on correctly. The non-obvious case is bool
// false: fmt.Sprintf("%v", false) yields "false", which Go's `if` treats
// as truthy (any non-empty string is truthy). Cardigann YAML expects
// false to be falsy, so we emit "" for false and "true" for true.
// Non-bool values pass through unchanged.
func configValueAsTemplateString(v interface{}) string {
	if b, ok := v.(bool); ok {
		if b {
			return "true"
		}
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// normalizeBoolString folds the string forms a UI checkbox might serialize
// ("false", "0", "no", "off") down to "" so they are falsy in Go templates.
// "true"/"1"/"yes"/"on" pass through as "true". Anything else is left as-is
// so non-checkbox config values (sort fields, category IDs, etc.) keep
// their literal value.
func normalizeBoolString(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "false", "0", "no", "off":
		return ""
	case "true", "1", "yes", "on":
		return "true"
	}
	return s
}

// buildQueryString assembles the URL query string from path-level inputs
// merged on top of search-block-level inputs (Cardigann convention:
// path inputs win on key collision). Each value is a Go template
// evaluated against tmplCtx — Nyaa.si's `q: "{{ .Keywords }}"` is the
// canonical example. Empty values are skipped so we don't send
// `&strip_s01=` and the like when a checkbox is unset.
//
// A template error is a hard error for the whole path, never a silent
// skip: dropping a failed `q` input is exactly how Nyaa's search degraded
// to its unfiltered homepage and polluted every search with 75 unrelated
// rows.
//
// The special `$raw` input (Cardigann convention) is an already-formatted
// query-string fragment (e.g. `s_cat=5000&s_cat=5070&`) and is spliced in
// verbatim rather than escaped as a parameter.
//
// Sorted iteration so the same inputs always produce the same URL —
// makes scraper logs greppable and avoids spurious diffs in tests.
func (r *Runner) buildQueryString(pathInputs map[string]string, tmplCtx *TemplateContext) (string, error) {
	merged := make(map[string]string, len(r.def.Search.Inputs)+len(pathInputs))
	for k, v := range r.def.Search.Inputs {
		merged[k] = v
	}
	for k, v := range pathInputs {
		merged[k] = v
	}
	if len(merged) == 0 {
		return "", nil
	}

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		evaluated, err := EvalTemplate(merged[k], tmplCtx)
		if err != nil {
			return "", fmt.Errorf("evaluating input %q: %w", k, err)
		}
		if evaluated == "" {
			continue
		}
		if k == "$raw" {
			if raw := strings.Trim(evaluated, "&"); raw != "" {
				parts = append(parts, raw)
			}
			continue
		}
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(evaluated))
	}
	return strings.Join(parts, "&"), nil
}

// siteOrigin is scheme://host of rawURL, or fallback if it has none.
func siteOrigin(rawURL, fallback string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fallback
	}
	return u.Scheme + "://" + u.Host
}
