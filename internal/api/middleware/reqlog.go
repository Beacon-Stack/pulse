package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// RequestLogger logs each HTTP request with method, path, status, and duration.
//
// The response is wrapped with chi's writer, which still lets a handler
// take over the connection: a plain wrapper hides that from the WebSocket
// upgrade, which then fails.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			logger.Debug("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"duration", time.Since(start).String(),
			)
		})
	}
}
