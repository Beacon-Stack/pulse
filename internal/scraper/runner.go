package scraper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"gopkg.in/yaml.v3"
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

// Runner executes searches against a single indexer using its YAML definition.
type Runner struct {
	def          *Definition
	client       *RateLimitedClient
	flaresolverr *FlareSolverr
	settings     map[string]string // user-provided config values
	baseURL      string            // primary site URL
	logger       *slog.Logger
}

// NewRunner creates a runner for the given definition.
func NewRunner(def *Definition, settings map[string]string, flaresolverr *FlareSolverr, logger *slog.Logger) *Runner {
	delay := time.Duration(def.RequestDelay) * time.Millisecond
	baseURL := ""
	if len(def.Links) > 0 {
		baseURL = strings.TrimRight(def.Links[0], "/")
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
// Sorted iteration so the same inputs always produce the same URL —
// makes scraper logs greppable and avoids spurious diffs in tests.
func (r *Runner) buildQueryString(pathInputs map[string]string, tmplCtx *TemplateContext) string {
	merged := make(map[string]string, len(r.def.Search.Inputs)+len(pathInputs))
	for k, v := range r.def.Search.Inputs {
		merged[k] = v
	}
	for k, v := range pathInputs {
		merged[k] = v
	}
	if len(merged) == 0 {
		return ""
	}

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		evaluated, err := EvalTemplate(merged[k], tmplCtx)
		if err != nil || evaluated == "" {
			continue
		}
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(evaluated))
	}
	return strings.Join(parts, "&")
}

// ResolveDownload fetches a detail page and extracts the actual download URL
// (magnet link or .torrent URL) using the definition's download selectors.
func (r *Runner) ResolveDownload(ctx context.Context, detailURL string) (string, error) {
	if len(r.def.Download.Selectors) == 0 {
		// No download selectors — the detail URL IS the download URL
		return detailURL, nil
	}

	// Apply CF session if available
	domain := extractDomainFromURL(detailURL)
	if r.flaresolverr != nil && domain != "" {
		if sess, ok := r.flaresolverr.GetSession(domain); ok {
			r.client.ApplyCFSession(detailURL, sess.ToHTTPCookies(), sess.userAgent)
		}
	}

	r.logger.Debug("scraper: fetching detail page for download",
		"indexer", r.def.Name,
		"url", detailURL,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, detailURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching detail page: %w", err)
	}
	defer resp.Body.Close()

	// If Cloudflare blocked, try FlareSolverr
	if (resp.StatusCode == 403 || resp.StatusCode == 503) && r.flaresolverr != nil {
		cfHeader := resp.Header.Get("Cf-Mitigated")
		server := resp.Header.Get("Server")
		if cfHeader == "challenge" || strings.Contains(strings.ToLower(server), "cloudflare") {
			resp.Body.Close()
			html, _, _, err := r.flaresolverr.Solve(ctx, detailURL)
			if err != nil {
				return "", fmt.Errorf("FlareSolverr failed for download page: %w", err)
			}
			return r.extractDownloadFromHTML(html, detailURL)
		}
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("detail page returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", fmt.Errorf("reading detail page: %w", err)
	}

	return r.extractDownloadFromHTML(string(body), detailURL)
}

// extractDownloadFromHTML applies download selectors to HTML and returns the first matching URL.
func (r *Runner) extractDownloadFromHTML(html, baseURL string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", fmt.Errorf("parsing detail page HTML: %w", err)
	}

	tmplCtx := NewTemplateContext(r.settings, "")

	for _, sel := range r.def.Download.Selectors {
		// Evaluate the selector template (some use {{ .Config.downloadlink }})
		selectorStr := sel.Selector
		if strings.Contains(selectorStr, "{{") {
			selectorStr = EvalTemplateOr(selectorStr, tmplCtx, selectorStr)
		}

		found := doc.Find(selectorStr)
		if found.Length() == 0 {
			continue
		}

		var link string
		if sel.Attribute != "" {
			link, _ = found.First().Attr(sel.Attribute)
		} else {
			link = strings.TrimSpace(found.First().Text())
		}

		if link == "" {
			continue
		}

		// Apply filters if any. tmplCtx is the same one used to evaluate
		// the selector itself, so download-link filters that include
		// {{ .Config.foo }} directives resolve against the user's
		// settings.
		if len(sel.Filters) > 0 {
			link = ApplyFilters(link, sel.Filters, tmplCtx)
		}

		// Make absolute if relative
		if !strings.HasPrefix(link, "http") && !strings.HasPrefix(link, "magnet:") {
			link = r.baseURL + "/" + strings.TrimLeft(link, "/")
		}

		return link, nil
	}

	return "", fmt.Errorf("no download link found on detail page")
}

// Search executes a search and returns structured results.
func (r *Runner) Search(ctx context.Context, query string, categories []int) ([]SearchResult, error) {
	// Apply keyword filters. We need a context for template evaluation —
	// keyword filters routinely conditionally rewrite the query based on
	// user settings (e.g. Nyaa's strip_s01 / radarr_compatibility).
	preCtx := NewTemplateContext(r.settings, query)
	for _, f := range r.def.Search.KeywordsFilters {
		query = applyFilter(query, f, preCtx)
	}

	// Map requested Torznab categories to site category IDs
	siteCats := r.mapCategories(categories)

	// Build template context
	tmplCtx := NewTemplateContext(r.settings, query)
	tmplCtx.Categories = siteCats

	var allResults []SearchResult

	// Execute each search path
	for _, path := range r.def.Search.Paths {
		results, err := r.executePath(ctx, path, tmplCtx)
		if err != nil {
			r.logger.Warn("scraper: search path failed",
				"indexer", r.def.Name,
				"path", path.Path,
				"error", err,
			)
			continue
		}
		allResults = append(allResults, results...)
	}

	return allResults, nil
}

// executePath fetches a single search URL and extracts results.
func (r *Runner) executePath(ctx context.Context, path SearchPath, tmplCtx *TemplateContext) ([]SearchResult, error) {
	// Evaluate the path template
	evalPath, err := EvalTemplate(path.Path, tmplCtx)
	if err != nil {
		return nil, fmt.Errorf("evaluating path template: %w", err)
	}

	// If the path is already an absolute URL, use it directly.
	var fullURL string
	if strings.HasPrefix(evalPath, "http://") || strings.HasPrefix(evalPath, "https://") {
		fullURL = evalPath
	} else {
		fullURL = r.baseURL + "/" + strings.TrimLeft(evalPath, "/")
	}

	// Append the query string built from path.Inputs and the
	// search-block-level inputs (search.inputs in YAML). Cardigann's
	// convention: path-level inputs override block-level inputs of the
	// same name. Without this, Nyaa.si requests hit `/` with no `?q=…`,
	// so the homepage comes back regardless of the search query.
	if qs := r.buildQueryString(path.Inputs, tmplCtx); qs != "" {
		sep := "?"
		if strings.Contains(fullURL, "?") {
			sep = "&"
		}
		fullURL += sep + qs
	}

	r.logger.Debug("scraper: fetching",
		"indexer", r.def.Name,
		"url", fullURL,
	)

	// Check for a cached FlareSolverr session for this domain and apply it.
	domain := extractDomainFromURL(fullURL)
	if r.flaresolverr != nil && domain != "" {
		if sess, ok := r.flaresolverr.GetSession(domain); ok {
			r.client.ApplyCFSession(fullURL, sess.ToHTTPCookies(), sess.userAgent)
		}
	}

	// Build request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 || resp.StatusCode == 503 {
		// Check for Cloudflare challenge
		cfHeader := resp.Header.Get("Cf-Mitigated")
		server := resp.Header.Get("Server")
		if cfHeader == "challenge" || strings.Contains(strings.ToLower(server), "cloudflare") {
			// Try FlareSolverr if available
			if r.flaresolverr != nil {
				resp.Body.Close()
				return r.solveWithFlareSolverr(ctx, fullURL, path, tmplCtx)
			}
			return nil, &CloudflareError{StatusCode: resp.StatusCode}
		}
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Determine response type
	respType := "html"
	if path.Response != nil && path.Response.Type != "" {
		respType = path.Response.Type
	}

	switch respType {
	case "json":
		return r.parseJSON(string(body), tmplCtx)
	default:
		return r.parseHTML(string(body), tmplCtx)
	}
}

// solveWithFlareSolverr uses FlareSolverr to bypass Cloudflare and returns
// parsed results from the solved page. The HTML from FlareSolverr is used
// directly — no need to re-fetch with cookies.
func (r *Runner) solveWithFlareSolverr(ctx context.Context, fullURL string, path SearchPath, tmplCtx *TemplateContext) ([]SearchResult, error) {
	html, _, _, err := r.flaresolverr.Solve(ctx, fullURL)
	if err != nil {
		return nil, fmt.Errorf("FlareSolverr failed: %w", err)
	}

	// Use the HTML directly from FlareSolverr's response.
	respType := "html"
	if path.Response != nil && path.Response.Type != "" {
		respType = path.Response.Type
	}

	switch respType {
	case "json":
		return r.parseJSON(html, tmplCtx)
	default:
		return r.parseHTML(html, tmplCtx)
	}
}

// parseHTML extracts results from an HTML response using CSS selectors.
func (r *Runner) parseHTML(body string, tmplCtx *TemplateContext) ([]SearchResult, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	// Evaluate the rows selector template (some use conditional logic)
	rowsDef := r.def.Search.Rows
	if strings.Contains(rowsDef.Selector, "{{") {
		evaluated := EvalTemplateOr(rowsDef.Selector, tmplCtx, rowsDef.Selector)
		rowsDef.Selector = evaluated
	}

	rows := FindRowsHTML(doc, rowsDef)

	var results []SearchResult
	rows.Each(func(_ int, row *goquery.Selection) {
		result := r.extractRow(row, tmplCtx)
		if result.Title != "" {
			results = append(results, result)
		}
	})

	return results, nil
}

// parseJSON extracts results from a JSON response.
func (r *Runner) parseJSON(body string, tmplCtx *TemplateContext) ([]SearchResult, error) {
	rowsDef := r.def.Search.Rows
	if strings.Contains(rowsDef.Selector, "{{") {
		evaluated := EvalTemplateOr(rowsDef.Selector, tmplCtx, rowsDef.Selector)
		rowsDef.Selector = evaluated
	}

	items := FindRowsJSON(body, rowsDef)
	r.logger.Debug("scraper: JSON parse",
		"indexer", r.def.Name,
		"rows_selector", rowsDef.Selector,
		"body_len", len(body),
		"items_found", len(items),
	)
	if len(items) > 0 {
		r.logger.Debug("scraper: first JSON item", "item", items[0][:min(200, len(items[0]))])
	}
	if items == nil {
		return nil, nil
	}

	var results []SearchResult
	for _, item := range items {
		result := r.extractJSONRow(item, tmplCtx)
		if result.Title != "" {
			results = append(results, result)
		}
	}

	return results, nil
}

// resultPassMaxIterations caps how many times pass 2 may re-run when
// chained .Result references (e.g. Nyaa's title_phase1 → title_phase2 →
// title_phase3 → title) need multiple iterations to stabilize. Empirically
// the longest chain in the Prowlarr-Indexers v11 set is ~6 hops; 16 is a
// generous ceiling that still bounds pathological cycles.
const resultPassMaxIterations = 16

// extractRow extracts a SearchResult from an HTML row.
func (r *Runner) extractRow(row *goquery.Selection, tmplCtx *TemplateContext) SearchResult {
	fieldValues := make(map[string]string)

	// Pass 1: fields without .Result references — they don't depend on
	// any other field, so a single pass in any order is sufficient.
	for name, field := range r.def.Search.Fields {
		if !strings.Contains(field.Text, ".Result.") {
			fieldValues[name] = ExtractFieldHTML(row, field, tmplCtx)
		}
	}
	tmplCtx.Result = fieldValues

	// Pass 2: fields with .Result references — iterate to a fixed point.
	// Cardigann YAML chains fields (e.g. Nyaa's title_phase1 →
	// title_phase2 → title_phase3 → title), and Go's map iteration order
	// is randomized, so a single pass can evaluate `title` before
	// `title_phase2` and produce "<no value>" output. Iterating until no
	// values change handles arbitrary DAG depths without us having to
	// build a topo sort.
	for iter := 0; iter < resultPassMaxIterations; iter++ {
		changed := false
		for name, field := range r.def.Search.Fields {
			if !strings.Contains(field.Text, ".Result.") {
				continue
			}
			newValue := ExtractFieldHTML(row, field, tmplCtx)
			if fieldValues[name] != newValue {
				fieldValues[name] = newValue
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	return r.buildResult(fieldValues)
}

// extractJSONRow extracts a SearchResult from a JSON item.
func (r *Runner) extractJSONRow(jsonItem string, tmplCtx *TemplateContext) SearchResult {
	fieldValues := make(map[string]string)

	for name, field := range r.def.Search.Fields {
		if !strings.Contains(field.Text, ".Result.") {
			fieldValues[name] = ExtractFieldJSON(jsonItem, field, tmplCtx)
		}
	}
	tmplCtx.Result = fieldValues

	// Same fixed-point iteration as the HTML path — see extractRow.
	for iter := 0; iter < resultPassMaxIterations; iter++ {
		changed := false
		for name, field := range r.def.Search.Fields {
			if !strings.Contains(field.Text, ".Result.") {
				continue
			}
			newValue := ExtractFieldJSON(jsonItem, field, tmplCtx)
			if fieldValues[name] != newValue {
				fieldValues[name] = newValue
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	return r.buildResult(fieldValues)
}

// buildResult converts extracted field values into a SearchResult.
func (r *Runner) buildResult(fields map[string]string) SearchResult {
	// Resolve details/download URLs
	details := fields["details"]
	if details != "" && !strings.HasPrefix(details, "http") {
		details = r.baseURL + "/" + strings.TrimLeft(details, "/")
	}

	download := fields["download"]
	if download != "" && !strings.HasPrefix(download, "http") && !strings.HasPrefix(download, "magnet:") {
		download = r.baseURL + "/" + strings.TrimLeft(download, "/")
	}

	magnetURI := fields["magneturi"]
	if magnetURI == "" && strings.HasPrefix(download, "magnet:") {
		magnetURI = download
	}

	size := parseSize(fields["size"])
	if fields["size"] != "" && size == 0 {
		r.logger.Debug("scraper: size parse failed", "raw_size", fields["size"], "indexer", r.def.Name)
	}
	seeders, _ := strconv.Atoi(strings.TrimSpace(fields["seeders"]))
	leechers, _ := strconv.Atoi(strings.TrimSpace(fields["leechers"]))
	grabs, _ := strconv.Atoi(strings.TrimSpace(fields["grabs"]))

	dvf := 1.0
	if v, err := strconv.ParseFloat(fields["downloadvolumefactor"], 64); err == nil {
		dvf = v
	}
	uvf := 1.0
	if v, err := strconv.ParseFloat(fields["uploadvolumefactor"], 64); err == nil {
		uvf = v
	}

	// Map category
	cat := fields["category"]
	catID := r.resolveCategoryID(cat)

	return SearchResult{
		Title:                fields["title"],
		Details:              details,
		Download:             download,
		MagnetURI:            magnetURI,
		InfoHash:             fields["infohash"],
		Size:                 size,
		Seeders:              seeders,
		Leechers:             leechers,
		Grabs:                grabs,
		Date:                 fields["date"],
		Category:             catID,
		DownloadVolumeFactor: dvf,
		UploadVolumeFactor:   uvf,
		Description:          fields["description"],
		IMDBID:               fields["imdbid"],
		Poster:               fields["poster"],
	}
}

// mapCategories maps Newznab category IDs to site-specific IDs.
func (r *Runner) mapCategories(newznabCats []int) []string {
	if len(newznabCats) == 0 {
		return nil
	}
	var siteCats []string
	for _, nc := range newznabCats {
		ncStr := strconv.Itoa(nc)
		for _, cm := range r.def.Caps.CategoryMappings {
			// Match if the Newznab cat prefix matches
			catParts := strings.Split(cm.Cat, "/")
			if len(catParts) > 0 {
				base := newznabBase(catParts[0])
				if base == ncStr || strings.HasPrefix(ncStr, base[:len(base)-1]) {
					siteCats = append(siteCats, cm.ID)
				}
			}
		}
	}
	return siteCats
}

// resolveCategoryID maps a site category ID back to a Newznab category ID string.
func (r *Runner) resolveCategoryID(siteCat string) string {
	for _, cm := range r.def.Caps.CategoryMappings {
		if cm.ID == siteCat {
			return newznabBase(strings.Split(cm.Cat, "/")[0])
		}
	}
	return "8000" // Other
}

// newznabBase returns the base Newznab category number for a category name.
func newznabBase(name string) string {
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

// sizeRe matches patterns like "1.5 GB", "797.7MB", "320 KiB" anywhere in a string.
var sizeRe = regexp.MustCompile(`(?i)([\d,.]+)[\s\x{00a0}]*(TIB|GIB|MIB|KIB|TB|GB|MB|KB|B)(?:[^A-Z]|$)`)

// parseSize converts human-readable size strings to bytes.
// Handles messy input like "797.7 MB3470" (size + seed count concatenated)
// and non-breaking spaces (U+00A0) used by some sites.
func parseSize(s string) int64 {
	s = strings.ReplaceAll(s, "\u00a0", " ") // normalize nbsp
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	multipliers := map[string]int64{
		"B": 1, "KB": 1024, "MB": 1024 * 1024, "GB": 1024 * 1024 * 1024, "TB": 1024 * 1024 * 1024 * 1024,
		"KIB": 1024, "MIB": 1024 * 1024, "GIB": 1024 * 1024 * 1024, "TIB": 1024 * 1024 * 1024 * 1024,
	}

	m := sizeRe.FindStringSubmatch(s)
	if len(m) == 3 {
		numStr := strings.ReplaceAll(m[1], ",", "")
		unit := strings.ToUpper(m[2])
		if mult, ok := multipliers[unit]; ok {
			f, err := strconv.ParseFloat(numStr, 64)
			if err == nil {
				return int64(f * float64(mult))
			}
		}
	}

	// Try parsing as raw number (bytes)
	n, _ := strconv.ParseInt(strings.ReplaceAll(s, ",", ""), 10, 64)
	return n
}

// ParseDefinition parses raw YAML bytes into a Definition.
//
// Applies a small set of upstream-typo workarounds before YAML parse —
// see normalizeCardigannQuirks. These exist because the Prowlarr-Indexers
// repo occasionally ships YAML with subtle template-syntax typos that
// Prowlarr's C# Cardigann implementation tolerates but Go's text/template
// rejects. We patch them at load time rather than per-call so the rest
// of the runner sees clean templates.
func ParseDefinition(raw []byte) (*Definition, error) {
	raw = normalizeCardigannQuirks(raw)
	var def Definition
	if err := yaml.Unmarshal(raw, &def); err != nil {
		return nil, fmt.Errorf("parsing definition YAML: %w", err)
	}
	return &def, nil
}

// normalizeCardigannQuirks repairs known upstream Cardigann YAML typos
// that Go's text/template can't tolerate. Each entry should be:
//   - documented with the indexer where it was first observed
//   - linked to the upstream YAML so we can drop the workaround once
//     Prowlarr-Indexers fixes it
//
// Replacements happen on the raw YAML bytes BEFORE yaml.Unmarshal so all
// downstream consumers (template parser, runner, tests) see the clean
// version. They are intentionally narrow string substitutions — broader
// regex normalization would risk corrupting valid templates that just
// happen to look adjacent to a known typo.
func normalizeCardigannQuirks(raw []byte) []byte {
	type quirk struct {
		from, to string
		// note is for code archaeology — link the upstream definition
		// where this typo lives so a future maintainer can verify it's
		// still needed before deleting the rule.
		note string
	}
	quirks := []quirk{
		{
			// 1337x v11 search path #2 (TV) has (eq … .False)) with one
			// extra closing paren. Movies/Music/Other paths in the same
			// file are correct.
			// https://github.com/Prowlarr/Indexers/blob/master/definitions/v11/1337x.yml
			from: ".False)) }}",
			to:   ".False) }}",
			note: "1337x.yml v11: TV-search path extra-paren typo",
		},
	}
	for _, q := range quirks {
		raw = bytes.ReplaceAll(raw, []byte(q.from), []byte(q.to))
	}
	return raw
}
