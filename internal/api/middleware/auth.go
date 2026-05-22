package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/ringmaster217/galactic-media-server/internal/auth"
)

type contextKey string

const claimsContextKey contextKey = "claims"

// Authenticate validates the Authorization: Bearer <token> header and stores
// the parsed claims in the request context. Returns 401 on failure.
func Authenticate(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := bearerToken(r)
			if tokenStr == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			claims, err := auth.ValidateAccessToken(jwtSecret, tokenStr)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin rejects requests from non-admin users with 403 Forbidden.
// Must be used after Authenticate in the middleware chain.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims == nil || !claims.IsAdmin {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ClaimsFromContext retrieves the JWT claims stored by Authenticate.
// Returns nil if the middleware has not run.
func ClaimsFromContext(ctx context.Context) *auth.Claims {
	v, _ := ctx.Value(claimsContextKey).(*auth.Claims)
	return v
}

// OptionalAuthenticate works like Authenticate but does not reject requests
// lacking a token. If a valid Bearer token is present its claims are stored in
// the context; otherwise the request proceeds without claims.
func OptionalAuthenticate(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := bearerToken(r)
			if tokenStr != "" {
				if claims, err := auth.ValidateAccessToken(jwtSecret, tokenStr); err == nil {
					ctx := context.WithValue(r.Context(), claimsContextKey, claims)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	// WebSocket clients cannot set custom headers; accept ?token= query param.
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return ""
}
