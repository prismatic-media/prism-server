package handler_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/ringmaster217/prism/internal/api/handler"
	apimw "github.com/ringmaster217/prism/internal/api/middleware"
	"github.com/ringmaster217/prism/internal/scanner"
	"github.com/ringmaster217/prism/pkg/events"
)

// artifactRouter creates a chi router with artifact handlers and RequireAdmin middleware.
func artifactRouter(db *sql.DB) http.Handler {
	indexer := scanner.NewIndexer(db, events.NewBus())
	artifactH := handler.NewArtifactHandler(db, indexer)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(apimw.Authenticate(testSecret))
	r.With(apimw.RequireAdmin).Get("/api/v1/admin/artifacts/status", artifactH.HandleStatus)
	r.With(apimw.RequireAdmin).Post("/api/v1/admin/artifacts/index", artifactH.HandleIndex)
	r.With(apimw.RequireAdmin).Post("/api/v1/admin/artifacts/relink", artifactH.HandleRelink)
	return r
}

// TestArtifactIndex_Unauthenticated tests that the index endpoint requires admin auth.
func TestArtifactIndex_Unauthenticated(t *testing.T) {
	db := openTestDB(t)
	router := artifactRouter(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/artifacts/index", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected non-200 status for unauthenticated request, got %d", w.Code)
	}
}

// TestArtifactIndex_Unauthorized tests that non-admin users cannot access the index endpoint.
func TestArtifactIndex_Unauthorized(t *testing.T) {
	db := openTestDB(t)
	user := createUser(t, db, "regular", "regular@example.com", "pass123", false)
	router := artifactRouter(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/artifacts/index", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, user.ID, false))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected non-200 status for non-admin user, got %d", w.Code)
	}
}

// TestArtifactIndex_AdminAuthorized tests that admin users can access the index endpoint.
func TestArtifactIndex_AdminAuthorized(t *testing.T) {
	db := openTestDB(t)
	admin := createUser(t, db, "admin", "admin@example.com", "pass123", true)
	router := artifactRouter(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/artifacts/index", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, admin.ID, true))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should succeed (or return 409 if schema not ready — but it should not return 401/403).
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("expected success for admin user, got %d", w.Code)
	}
}

// TestArtifactStatus_Unauthenticated tests that the status endpoint requires admin auth.
func TestArtifactStatus_Unauthenticated(t *testing.T) {
	db := openTestDB(t)
	router := artifactRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/artifacts/status", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected non-200 status for unauthenticated request, got %d", w.Code)
	}
}

// TestArtifactStatus_Unauthorized tests that non-admin users cannot access the status endpoint.
func TestArtifactStatus_Unauthorized(t *testing.T) {
	db := openTestDB(t)
	user := createUser(t, db, "regular", "regular@example.com", "pass123", false)
	router := artifactRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/artifacts/status", nil)
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, user.ID, false))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected non-200 status for non-admin user, got %d", w.Code)
	}
}

// TestArtifactStatus_AdminAuthorized tests that admin users can access the status endpoint.
func TestArtifactStatus_AdminAuthorized(t *testing.T) {
	db := openTestDB(t)
	admin := createUser(t, db, "admin", "admin@example.com", "pass123", true)
	router := artifactRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/artifacts/status", nil)
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, admin.ID, true))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should succeed (returns health counts, even if empty).
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for admin user, got %d: %s", w.Code, w.Body.String())
	}

	// Verify response is valid JSON.
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}
}

// TestArtifactRelink_Unauthenticated tests that the relink endpoint requires admin auth.
func TestArtifactRelink_Unauthenticated(t *testing.T) {
	db := openTestDB(t)
	router := artifactRouter(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/artifacts/relink", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected non-200 status for unauthenticated request, got %d", w.Code)
	}
}

// TestArtifactRelink_Unauthorized tests that non-admin users cannot access the relink endpoint.
func TestArtifactRelink_Unauthorized(t *testing.T) {
	db := openTestDB(t)
	user := createUser(t, db, "regular", "regular@example.com", "pass123", false)
	router := artifactRouter(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/artifacts/relink", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, user.ID, false))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected non-200 status for non-admin user, got %d", w.Code)
	}
}

// TestArtifactRelink_AdminAuthorized tests that admin users can access the relink endpoint.
func TestArtifactRelink_AdminAuthorized(t *testing.T) {
	db := openTestDB(t)
	admin := createUser(t, db, "admin", "admin@example.com", "pass123", true)
	router := artifactRouter(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/artifacts/relink", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, admin.ID, true))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should succeed (returns relink summary, even if empty).
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for admin user, got %d: %s", w.Code, w.Body.String())
	}

	// Verify response is valid JSON.
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}
}

// helper to generate a valid UUID for test DB
var _ = uuid.New
