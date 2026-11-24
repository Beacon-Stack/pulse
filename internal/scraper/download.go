package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

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
			// Detached for the same reason as solveWithFlareSolverr: let
			// the solve complete and cache the session even if the
			// download caller gives up first.
			solveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flareSolveBudget)
			defer cancel()
			html, _, _, err := r.flaresolverr.Solve(solveCtx, detailURL)
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

		// Make absolute if relative, against the detail page's own site
		// (it may be a mirror).
		if !strings.HasPrefix(link, "http") && !strings.HasPrefix(link, "magnet:") {
			link = siteOrigin(baseURL, r.baseURL) + "/" + strings.TrimLeft(link, "/")
		}

		return link, nil
	}

	return "", fmt.Errorf("no download link found on detail page")
}
