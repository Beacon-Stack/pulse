// Package web serves the embedded Pulse web UI.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static
var staticFiles embed.FS

// ServeStatic returns an http.Handler that serves the React SPA.
// /assets/* are cached aggressively; unknown paths fall back to index.html
// for React Router client-side navigation.
//
// When web/static/index.html is missing - the case in CI (the frontend
// build artifact is gitignored) and in backend-only dev runs - the
// returned handler responds with a small HTML stub explaining that the
// frontend has not been built. The API endpoints registered elsewhere
// on the router still work normally; only the SPA shell is degraded.
// This keeps the test suite and bare-metal dev runs from panicking at
// router construction.
func ServeStatic() http.Handler {
	indexHTML, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		return frontendNotBuiltHandler(err)
	}

	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return frontendNotBuiltHandler(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path)

		if strings.HasPrefix(p, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			fileServer.ServeHTTP(w, r)
			return
		}

		if p != "/" && p != "/index.html" {
			f, err := sub.Open(strings.TrimPrefix(p, "/"))
			if err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(indexHTML)
	})
}

// frontendNotBuiltHandler is the fallback when web/static/index.html is
// missing from the embedded FS. Returns a 503 with a short HTML body so
// any browser that does hit a SPA path gets a clear message; API routes
// registered on the same router continue to work.
func frontendNotBuiltHandler(reason error) http.Handler {
	body := []byte(`<!DOCTYPE html>
<html><body style="font-family: system-ui; padding: 2rem;">
<h1>Pulse - frontend not built</h1>
<p>The server is running but the React UI bundle is missing.
Run <code>npm --prefix web/ui run build</code> (or use the published Docker image) to populate <code>web/static/</code>.</p>
<p style="color:#666;font-size:0.9em">` + http.StatusText(http.StatusServiceUnavailable) + ` - ` + reason.Error() + `</p>
</body></html>`)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write(body)
	})
}
