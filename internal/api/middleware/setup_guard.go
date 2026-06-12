package middleware

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/ringmaster217/prism/internal/store/sqlite"
)

// SetupGuard intercepts all requests when setup is not yet complete and
// redirects the client to /setup. Requests to /setup and /api/v1/setup*
// are always allowed through to avoid redirect loops.
func SetupGuard(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Always allow setup-related paths and static assets through.
			if isSetupPath(r.URL.Path) || isStaticAsset(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			done, err := sqlite.GetSetting(r.Context(), db, "setup_complete")
			if err != nil && err != sqlite.ErrNotFound {
				// If we can't read the setting, fail open (don't block the app).
				next.ServeHTTP(w, r)
				return
			}

			if done != "true" {
				// API clients get a JSON 503; browser requests get a redirect.
				if strings.HasPrefix(r.URL.Path, "/api/") {
					http.Error(w, `{"error":"server setup required","redirect":"/setup"}`, http.StatusServiceUnavailable)
					return
				}
				http.Redirect(w, r, "/setup", http.StatusTemporaryRedirect)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isSetupPath(path string) bool {
	return path == "/setup" ||
		strings.HasPrefix(path, "/setup/") ||
		path == "/api/v1/setup" ||
		strings.HasPrefix(path, "/api/v1/setup/") ||
		path == "/api/v1/fs/browse"
}

// isStaticAsset returns true for file extensions that are always safe to serve
// regardless of setup state. The Angular app must be able to load its JS/CSS
// bundles to render the setup wizard in the first place.
func isStaticAsset(path string) bool {
	for _, ext := range []string{".js", ".css", ".ico", ".png", ".svg", ".woff", ".woff2", ".ttf", ".map", ".webp", ".jpg", ".jpeg"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// AuthenticateSetupOrAdmin verifies either that setup is not yet complete,
// or that a valid admin JWT token is present.
func AuthenticateSetupOrAdmin(db *sql.DB, jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			done, err := sqlite.GetSetting(r.Context(), db, "setup_complete")
			if err == nil && done != "true" {
				// Setup is not complete, allow unauthenticated access.
				next.ServeHTTP(w, r)
				return
			}

			// Setup is complete, require normal admin authentication.
			Authenticate(jwtSecret)(RequireAdmin(next)).ServeHTTP(w, r)
		})
	}
}
