package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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
		fullURL = tmplCtx.base(r.baseURL) + "/" + strings.TrimLeft(evalPath, "/")
	}

	// Append the query string built from path.Inputs and the
	// search-block-level inputs (search.inputs in YAML). Cardigann's
	// convention: path-level inputs override block-level inputs of the
	// same name. Without this, Nyaa.si requests hit `/` with no `?q=…`,
	// so the homepage comes back regardless of the search query.
	qs, err := r.buildQueryString(path.Inputs, tmplCtx)
	if err != nil {
		return nil, err
	}
	if qs != "" {
		sep := "?"
		if strings.Contains(fullURL, "?") {
			sep = "&"
		}
		fullURL += sep + qs
	}

	// Re-encode the query portion. Keywords substituted into the path
	// template itself (`search?q={{ .Keywords }}`) arrive with raw spaces;
	// Go percent-encodes the *path* automatically at request time but
	// emits the query verbatim, so `GET /search?q=Severance S02E01` went
	// out with a literal space and servers rejected it with HTTP 400 —
	// this single defect broke TorrentDownload, kickasstorrents.to,
	// Torrent[CORE], and The Pirate Bay.
	u, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("invalid search URL %q: %w", fullURL, err)
	}
	u.RawQuery = u.Query().Encode()
	fullURL = u.String()

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

	// Definition-supplied headers (search.headers, overridden by
	// path-level headers). Values are templates.
	for k, vals := range r.def.Search.Headers {
		if len(vals) > 0 {
			req.Header.Set(k, EvalTemplateOr(vals[0], tmplCtx, vals[0]))
		}
	}
	for k, vals := range path.Headers {
		if len(vals) > 0 {
			req.Header.Set(k, EvalTemplateOr(vals[0], tmplCtx, vals[0]))
		}
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
			traceFrom(ctx).noteChallenge(false)
			return nil, &CloudflareError{StatusCode: resp.StatusCode}
		}
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode}
	}

	if resp.StatusCode != 200 {
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode}
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

// flareSolveBudget bounds a detached challenge solve. Matches
// FlareSolverr's own max solve timeout plus transport headroom.
const flareSolveBudget = 160 * time.Second

// solveWithFlareSolverr uses FlareSolverr to bypass Cloudflare and returns
// parsed results from the solved page. The HTML from FlareSolverr is used
// directly — no need to re-fetch with cookies.
//
// The solve runs detached from the caller's context: solves take 60-120s
// while impatient clients (Pilot's per-indexer timeout is 25s) hang up
// long before that. Propagating that cancellation aborted the solve
// mid-flight, so the session was never cached and every subsequent
// search hit a cold session again — CF-walled indexers were stuck in a
// permanent timeout loop. Letting the solve finish caches the session,
// making the NEXT search fast even when THIS caller has given up.
func (r *Runner) solveWithFlareSolverr(ctx context.Context, fullURL string, path SearchPath, tmplCtx *TemplateContext) ([]SearchResult, error) {
	solveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flareSolveBudget)
	defer cancel()
	html, _, _, err := r.flaresolverr.Solve(solveCtx, fullURL)
	traceFrom(ctx).noteChallenge(err == nil)
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
