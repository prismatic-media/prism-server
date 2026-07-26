package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/prismatic-media/prism-server/internal/api/handler"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

func TestServeActorImage(t *testing.T) {
	db := openTestDB(t)

	thumbsDir := t.TempDir()
	if err := sqlite.SetSetting(context.Background(), db, "thumbs_dir", thumbsDir); err != nil {
		t.Fatalf("setting thumbs_dir: %v", err)
	}

	actorFile := filepath.Join(thumbsDir, "actor_testactor.jpg")
	content := []byte("ACTOR_PHOTO_DATA")
	if err := os.WriteFile(actorFile, content, 0o644); err != nil {
		t.Fatalf("writing actor image file: %v", err)
	}

	h := handler.NewActorHandler(db)
	r := chi.NewRouter()
	r.Get("/api/v1/actors/image/{filename}", h.ServeActorImage)

	// Test 1: Successful fetch
	req := httptest.NewRequest(http.MethodGet, "/api/v1/actors/image/testactor.jpg", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
	if rec.Body.String() != string(content) {
		t.Errorf("expected body %q, got %q", content, rec.Body.String())
	}

	// Test 2: Non-existent image file
	req404 := httptest.NewRequest(http.MethodGet, "/api/v1/actors/image/nonexistent.jpg", nil)
	rec404 := httptest.NewRecorder()
	r.ServeHTTP(rec404, req404)

	if rec404.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", rec404.Code)
	}

	// Test 3: Path traversal rejection
	reqBad := httptest.NewRequest(http.MethodGet, "/api/v1/actors/image/..%2Fsecret.txt", nil)
	recBad := httptest.NewRecorder()
	r.ServeHTTP(recBad, reqBad)

	if recBad.Code != http.StatusBadRequest && recBad.Code != http.StatusNotFound {
		t.Errorf("expected 400 or 404 for path traversal attempt, got %d", recBad.Code)
	}
}
