package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Timeout is a middleware that wraps the request context with a timeout.
// Excludes WebSocket endpoints and endpoints carrying large files where connection duration is long-lived.
func Timeout(duration time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip websocket, streaming, download, source download, and bundle upload endpoints
			if r.Header.Get("Upgrade") == "websocket" ||
				strings.Contains(r.URL.Path, "/ws/") ||
				strings.Contains(r.URL.Path, "/stream/") ||
				strings.HasSuffix(r.URL.Path, "/download") ||
				strings.HasSuffix(r.URL.Path, "/source") ||
				strings.HasSuffix(r.URL.Path, "/bundle") {
				next.ServeHTTP(w, r)
				return
			}

			ctx, cancel := context.WithTimeout(r.Context(), duration)
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
