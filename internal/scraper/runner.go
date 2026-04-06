package scraper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

	// Merge settings with defaults from the definition
	merged := make(map[string]string)
	for _, s := range def.Settings {
		if s.Default != nil {
			merged[s.Name] = fmt.Sprintf("%v", s.Default)
		}
	}
	for k, v := range settings {
		merged[k] = v
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

		// Apply filters if any
		if len(sel.Filters) > 0 {
			link = ApplyFilters(link, sel.Filters)
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
	// Apply keyword filters
	for _, f := range r.def.Search.KeywordsFilters {
		query = applyFilter(query, f)
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

// extractRow extracts a SearchResult from an HTML row.
func (r *Runner) extractRow(row *goquery.Selection, tmplCtx *TemplateContext) SearchResult {
	// Extract all fields into the Result map so templates can reference them
	fieldValues := make(map[string]string)

	// Process fields in order — some fields reference others via .Result
	// First pass: fields without .Result references
	for name, field := range r.def.Search.Fields {
		if !strings.Contains(field.Text, ".Result.") {
			fieldValues[name] = ExtractFieldHTML(row, field, tmplCtx)
		}
	}
	tmplCtx.Result = fieldValues

	// Second pass: fields with .Result references
	for name, field := range r.def.Search.Fields {
		if strings.Contains(field.Text, ".Result.") {
			fieldValues[name] = ExtractFieldHTML(row, field, tmplCtx)
		}
	}
	tmplCtx.Result = fieldValues

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

	for name, field := range r.def.Search.Fields {
		if strings.Contains(field.Text, ".Result.") {
			fieldValues[name] = ExtractFieldJSON(jsonItem, field, tmplCtx)
		}
	}
	tmplCtx.Result = fieldValues

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

// parseSize converts human-readable size strings to bytes.
func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	multipliers := map[string]int64{
		"B": 1, "KB": 1024, "MB": 1024 * 1024, "GB": 1024 * 1024 * 1024, "TB": 1024 * 1024 * 1024 * 1024,
		"KIB": 1024, "MIB": 1024 * 1024, "GIB": 1024 * 1024 * 1024, "TIB": 1024 * 1024 * 1024 * 1024,
	}

	s = strings.ToUpper(strings.ReplaceAll(s, ",", ""))
	for suffix, mult := range multipliers {
		if strings.HasSuffix(s, suffix) {
			numStr := strings.TrimSpace(strings.TrimSuffix(s, suffix))
			f, err := strconv.ParseFloat(numStr, 64)
			if err == nil {
				return int64(f * float64(mult))
			}
		}
	}

	// Try parsing as raw number (bytes)
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// ParseDefinition parses raw YAML bytes into a Definition.
func ParseDefinition(raw []byte) (*Definition, error) {
	var def Definition
	if err := yaml.Unmarshal(raw, &def); err != nil {
		return nil, fmt.Errorf("parsing definition YAML: %w", err)
	}
	return &def, nil
}
