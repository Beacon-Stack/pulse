package scraper

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

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

	return r.buildResultAt(tmplCtx.base(r.baseURL), fieldValues)
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

	return r.buildResultAt(tmplCtx.base(r.baseURL), fieldValues)
}

// defaultMagnetTrackers is the announce list embedded in magnet links
// synthesized from an infohash-only search result. Mirrors the open
// tracker set Prowlarr bakes into its generated magnets — the swarm is
// also reachable via DHT, so this list is a bootstrap aid, not a
// requirement.
var defaultMagnetTrackers = []string{
	"udp://tracker.opentrackr.org:1337/announce",
	"udp://open.demonii.com:1337/announce",
	"udp://open.stealth.si:80/announce",
	"udp://tracker.torrent.eu.org:451/announce",
	"udp://exodus.desync.com:6969/announce",
}

// magnetFromInfoHash builds a magnet URI for definitions that yield an
// infohash but no download link (e.g. The Pirate Bay's apibay API).
func magnetFromInfoHash(infoHash, title string) string {
	var b strings.Builder
	b.WriteString("magnet:?xt=urn:btih:")
	b.WriteString(strings.TrimSpace(infoHash))
	if title != "" {
		b.WriteString("&dn=")
		b.WriteString(url.QueryEscape(title))
	}
	for _, tr := range defaultMagnetTrackers {
		b.WriteString("&tr=")
		b.WriteString(url.QueryEscape(tr))
	}
	return b.String()
}

// buildResult converts extracted field values into a SearchResult, with
// relative links resolved against the primary address.
func (r *Runner) buildResult(fields map[string]string) SearchResult {
	return r.buildResultAt(r.baseURL, fields)
}

// buildResultAt is buildResult for results that came from address base.
func (r *Runner) buildResultAt(base string, fields map[string]string) SearchResult {
	// Resolve details/download URLs
	details := fields["details"]
	if details != "" && !strings.HasPrefix(details, "http") {
		details = base + "/" + strings.TrimLeft(details, "/")
	}

	download := fields["download"]
	if download != "" && !strings.HasPrefix(download, "http") && !strings.HasPrefix(download, "magnet:") {
		download = base + "/" + strings.TrimLeft(download, "/")
	}

	// Cardigann YAML defs in the wild use either `magneturi` or
	// `magnet` for the magnet selector (Prowlarr's nyaasi.yml,
	// sukebei.yml, tokyotoshokan.yml all use `magnet`). Accept both
	// — without this, Nyaa torznab responses ship with no
	// <enclosure> tag and grabbing fails downstream with "release …
	// has no download URL".
	magnetURI := fields["magneturi"]
	if magnetURI == "" {
		magnetURI = fields["magnet"]
	}
	if magnetURI == "" && strings.HasPrefix(download, "magnet:") {
		magnetURI = download
	}
	// Some definitions provide only an infohash — Prowlarr's
	// thepiratebay.yml is the canonical case: apibay serves no
	// .torrent files and the YAML has no download/magnet field, so
	// Prowlarr's engine synthesizes a magnet link from the hash.
	// Match that behavior; without it every TPB row ships with no
	// <enclosure> and grabbing fails downstream with "release … has
	// no download URL".
	if magnetURI == "" && download == "" && fields["infohash"] != "" {
		magnetURI = magnetFromInfoHash(fields["infohash"], fields["title"])
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
