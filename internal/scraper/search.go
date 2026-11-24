package scraper

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/beacon-stack/pulse/internal/sitenet"
)

// SearchQuery carries the structured torznab search parameters into the
// template context, so definitions using .Query.Season / .Query.IMDBID
// and friends resolve correctly.
type SearchQuery struct {
	Keywords   string
	Type       string // torznab t= mode: search, tvsearch, movie, ...
	Season     int
	Ep         int
	IMDBID     string
	TMDBID     string
	TVDBID     string
	Year       int
	Categories []int
}

// Search executes a search and returns structured results.
// Kept for callers that only have a bare keyword string.
func (r *Runner) Search(ctx context.Context, query string, categories []int) ([]SearchResult, error) {
	return r.SearchWithQuery(ctx, SearchQuery{Keywords: query, Categories: categories})
}

// SearchWithQuery executes a search with full structured parameters.
//
// Error contract: per-path failures are collected; if at least one path
// produced results, those are returned (with failures logged). If EVERY
// path failed, the combined error is returned — silently degrading a
// hard failure to "0 results" is what let half the indexer fleet stay
// broken for weeks without anything surfacing it.
func (r *Runner) SearchWithQuery(ctx context.Context, sq SearchQuery) ([]SearchResult, error) {
	query := sq.Keywords

	// Apply keyword filters. We need a context for template evaluation —
	// keyword filters routinely conditionally rewrite the query based on
	// user settings (e.g. Nyaa's strip_s01 / radarr_compatibility).
	preCtx := NewTemplateContext(r.settings, query)
	for _, f := range r.def.Search.KeywordsFilters {
		query = applyFilter(query, f, preCtx)
	}

	// Map requested Torznab categories to site category IDs
	siteCats := r.mapCategories(sq.Categories)

	// Build template context
	tmplCtx := NewTemplateContext(r.settings, query)
	tmplCtx.Categories = siteCats
	tmplCtx.Query.Type = sq.Type
	if sq.Season > 0 {
		tmplCtx.Query.Season = strconv.Itoa(sq.Season)
	}
	if sq.Ep > 0 {
		tmplCtx.Query.Ep = strconv.Itoa(sq.Ep)
	}
	tmplCtx.Query.IMDBID = sq.IMDBID
	tmplCtx.Query.TMDBID = sq.TMDBID
	tmplCtx.Query.TVDBID = sq.TVDBID
	if sq.Year > 0 {
		tmplCtx.Query.Year = strconv.Itoa(sq.Year)
	}

	// Try the site's current address, then its mirrors, while every
	// search page fails for a site-level reason (see mirrorWorthy).
	bases := r.baseOrder()
	var firstErr error
	for i, base := range bases {
		results, pathErrs := r.searchAt(ctx, base, query, tmplCtx)
		if len(results) > 0 || len(pathErrs) == 0 {
			if i > 0 {
				r.useBase(base)
				r.logger.Info("scraper: site answered on another address",
					"indexer", r.def.Name, "address", base)
			}
			return r.applyRowsFilters(results, query), nil
		}
		err := errors.Join(pathErrs...)
		if firstErr == nil {
			firstErr = err
		}
		if ctx.Err() != nil || !allMirrorWorthy(pathErrs) || i == len(bases)-1 {
			if i == 0 {
				return nil, err
			}
			return nil, fmt.Errorf("%w (also tried %d other site address(es))", firstErr, i)
		}
		r.logger.Warn("scraper: site address failed, trying the next",
			"indexer", r.def.Name, "address", base, "next", bases[i+1], "error", err)
	}
	return nil, firstErr
}

// searchAt runs every search path against one site address. Paths are
// labelled "search page N" in errors (the raw templates are long and
// pushed the real cause out of truncated messages).
func (r *Runner) searchAt(ctx context.Context, base, query string, proto *TemplateContext) ([]SearchResult, []error) {
	tmplCtx := *proto
	tmplCtx.siteBase = base
	if base != r.baseURL {
		cfg := make(map[string]string, len(proto.Config))
		for k, v := range proto.Config {
			cfg[k] = v
		}
		cfg["sitelink"] = base + "/"
		tmplCtx.Config = cfg
	}
	var allResults []SearchResult
	var pathErrs []error
	for i, path := range r.def.Search.Paths {
		results, err := r.executePath(ctx, path, &tmplCtx)
		if err != nil {
			r.logger.Warn("scraper: search path failed",
				"indexer", r.def.Name,
				"address", base,
				"path", path.Path,
				"error", err,
			)
			if len(r.def.Search.Paths) > 1 {
				err = fmt.Errorf("search page %d: %w", i+1, err)
			}
			pathErrs = append(pathErrs, err)
			continue
		}
		allResults = append(allResults, results...)
	}
	return allResults, pathErrs
}

// baseOrder lists the site's addresses to try: the one that last worked,
// then the others in definition order.
func (r *Runner) baseOrder() []string {
	if len(r.links) == 0 {
		return []string{r.baseURL}
	}
	active := int(r.active.Load())
	if active < 0 || active >= len(r.links) {
		active = 0
	}
	order := []string{r.links[active]}
	for i, l := range r.links {
		if i != active {
			order = append(order, l)
		}
	}
	return order
}

// useBase remembers which address worked, so the next search starts there.
func (r *Runner) useBase(base string) {
	for i, l := range r.links {
		if l == base {
			r.active.Store(int32(i))
			return
		}
	}
}

// CurrentBase is the site address searches start with: the one that last
// worked, else the definition's first link.
func (r *Runner) CurrentBase() string {
	return r.baseOrder()[0]
}

// mirrorWorthy reports whether an error means "this address doesn't work"
// rather than "this search failed": no DNS answer, a refused, reset or
// timed-out connection, the VPN proxy unable to reach the site, HTTP 410,
// or a 5xx. Not Cloudflare (mirrors share it), not other 4xx, and not our
// own proxy being down.
func mirrorWorthy(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || sitenet.IsProxyError(err) || IsCloudflareError(err) {
		return false
	}
	var status *HTTPStatusError
	if errors.As(err, &status) {
		return status.StatusCode == http.StatusGone || status.StatusCode >= 500
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

func allMirrorWorthy(errs []error) bool {
	for _, err := range errs {
		if !mirrorWorthy(err) {
			return false
		}
	}
	return len(errs) > 0
}

// applyRowsFilters applies definition-level rows filters. Currently
// implements `andmatch`: keep only rows whose title contains every
// keyword term (case-insensitive) — Cardigann's client-side guard against
// sites whose search fuzzy-matches or ignores the query.
func (r *Runner) applyRowsFilters(results []SearchResult, keywords string) []SearchResult {
	hasAndMatch := false
	for _, f := range r.def.Search.Rows.Filters {
		if strings.EqualFold(f.Name, "andmatch") {
			hasAndMatch = true
		}
	}
	if !hasAndMatch || strings.TrimSpace(keywords) == "" {
		return results
	}

	terms := strings.Fields(strings.ToLower(keywords))
	filtered := results[:0]
	for _, res := range results {
		title := strings.ToLower(res.Title)
		match := true
		for _, t := range terms {
			if !strings.Contains(title, t) {
				match = false
				break
			}
		}
		if match {
			filtered = append(filtered, res)
		}
	}
	return filtered
}
