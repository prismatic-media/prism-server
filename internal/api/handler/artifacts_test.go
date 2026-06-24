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

	"github.com/prismatic-media/prism-server/internal/api/handler"
	apimw "github.com/prismatic-media/prism-server/internal/api/middleware"
	"github.com/prismatic-media/prism-server/internal/scanner"
	"github.com/prismatic-media/prism-server/pkg/events"
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
	r.With(apimw.RequireAdmin).Post("/api/v1/admin/artifacts/regenerate-mpds", artifactH.HandleRegenerateMPDs)
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

func TestArtifactRegenerateMPDs_Auth(t *testing.T) {
	db := openTestDB(t)
	router := artifactRouter(db)

	// 1. Unauthenticated
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/artifacts/regenerate-mpds", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	if w1.Code == http.StatusOK {
		t.Errorf("expected auth block, got status %d", w1.Code)
	}

	// 2. Non-admin user
	user := createUser(t, db, "regular", "regular@example.com", "pass123", false)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/artifacts/regenerate-mpds", nil)
	req2.Header.Set("Authorization", "Bearer "+bearerToken(t, user.ID, false))
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code == http.StatusOK {
		t.Errorf("expected forbidden for non-admin, got status %d", w2.Code)
	}

	// 3. Admin user (succeeds)
	admin := createUser(t, db, "admin", "admin@example.com", "pass123", true)
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/artifacts/regenerate-mpds", nil)
	req3.Header.Set("Authorization", "Bearer "+bearerToken(t, admin.ID, true))
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Errorf("expected 200 for admin user, got %d: %s", w3.Code, w3.Body.String())
	}
}

func TestArtifactRegenerateMPDs_Success(t *testing.T) {
	db := openTestDB(t)
	admin := createUser(t, db, "admin", "admin@example.com", "pass123", true)
	router := artifactRouter(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/artifacts/regenerate-mpds", nil)
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, admin.ID, true))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 status, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if val, ok := resp["regenerated"]; !ok || val != float64(0) {
		t.Errorf("expected regenerated = 0, got %v", val)
	}
	if val, ok := resp["errors"]; !ok || val != float64(0) {
		t.Errorf("expected errors = 0, got %v", val)
	}
}

// helper to generate a valid UUID for test DB
var _ = uuid.New
