package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/prismatic-media/prism-server/internal/api/handler"
	apimw "github.com/prismatic-media/prism-server/internal/api/middleware"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/pkg/dash"
)

// --- helpers ---

func routerWithStream(db interface {
	// type-assert to *sql.DB in the actual builder
}, segDir string, cache *dash.Cache) http.Handler {
	return nil // placeholder — not used; tests use the handler directly below
}

// newStreamRouter builds a minimal chi router wired to StreamHandler.
func newStreamRouter(t *testing.T, h *handler.StreamHandler) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	r.Use(apimw.Authenticate(testSecret))
	r.Get("/api/v1/stream/{id}/manifest.mpd", h.ServeManifest)
	r.Get("/api/v1/stream/{id}/segments/*", h.ServeSegment)
	return r
}

// newHistoryRouter builds a minimal chi router wired to HistoryHandler.
func newHistoryRouter(t *testing.T, h *handler.HistoryHandler) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	r.Use(apimw.Authenticate(testSecret))
	r.Get("/api/v1/history", h.GetHistory)
	r.Put("/api/v1/history/{mediaID}", h.UpsertHistory)
	return r
}

// --- StreamHandler tests ---

func TestServeManifest_MediaNotFound(t *testing.T) {
	db := openTestDB(t)
	cache := &dash.Cache{}
	h := handler.NewStreamHandler(db, cache, testSecret)
	r := newStreamRouter(t, h)

	u := createUser(t, db, "user1", "u1@x.com", "pass", false)
	tok := bearerToken(t, u.ID, false)

	rec := do(t, r, http.MethodGet, "/api/v1/stream/00000000-0000-0000-0000-000000000001/manifest.mpd",
		nil, map[string]string{"Authorization": "Bearer " + tok})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestServeManifest_TranscodePending(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := &models.MediaItem{
		LibraryID: lib.ID, Title: "Film", FilePath: "/l/f.mkv",
		MediaType: models.MediaTypeMovie, TranscodeStatus: models.TranscodeStatusPending,
	}
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}
	// No MPDPath set — still pending.

	cache := &dash.Cache{} // empty
	h := handler.NewStreamHandler(db, cache, testSecret)
	r := newStreamRouter(t, h)

	u := createUser(t, db, "user2", "u2@x.com", "pass", false)
	tok := bearerToken(t, u.ID, false)

	rec := do(t, r, http.MethodGet, "/api/v1/stream/"+m.ID.String()+"/manifest.mpd",
		nil, map[string]string{"Authorization": "Bearer " + tok})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestServeManifest_FromCache(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := &models.MediaItem{
		LibraryID: lib.ID, Title: "Film", FilePath: "/l/f.mkv",
		MediaType: models.MediaTypeMovie, TranscodeStatus: models.TranscodeStatusPending,
	}
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	// Write a real MPD file for http.ServeFile to return.
	segDir := t.TempDir()
	mpdPath := filepath.Join(segDir, "test.mpd")
	if err := os.WriteFile(mpdPath, []byte(`<?xml version="1.0"?>`), 0o644); err != nil {
		t.Fatal(err)
	}

	cache := &dash.Cache{}
	cache.Set(m.ID, mpdPath)

	h := handler.NewStreamHandler(db, cache, testSecret)
	r := newStreamRouter(t, h)

	u := createUser(t, db, "user3", "u3@x.com", "pass", false)
	tok := bearerToken(t, u.ID, false)

	rec := do(t, r, http.MethodGet, "/api/v1/stream/"+m.ID.String()+"/manifest.mpd",
		nil, map[string]string{"Authorization": "Bearer " + tok})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/dash+xml" {
		t.Errorf("Content-Type = %q, want application/dash+xml", ct)
	}
}

func TestServeSegment_PathTraversal(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := &models.Library{Path: "/l-seg-path", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := &models.MediaItem{
		LibraryID: lib.ID, Title: "Film", FilePath: "/l-seg-path/f.mkv",
		MediaType: models.MediaTypeMovie, TranscodeStatus: models.TranscodeStatusDone,
	}
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}
	mpdPath := filepath.Join(t.TempDir(), "manifest.mpd")
	if err := os.WriteFile(mpdPath, []byte("<MPD/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.SetMediaMPDPath(ctx, db, m.ID, mpdPath); err != nil {
		t.Fatal(err)
	}

	cache := &dash.Cache{}
	h := handler.NewStreamHandler(db, cache, testSecret)
	r := newStreamRouter(t, h)

	u := createUser(t, db, "user4", "u4@x.com", "pass", false)
	tok := bearerToken(t, u.ID, false)

	rec := do(t, r, http.MethodGet,
		"/api/v1/stream/"+m.ID.String()+"/segments/../../etc/passwd",
		nil, map[string]string{"Authorization": "Bearer " + tok})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (path traversal rejected)", rec.Code)
	}
}

func TestServeSegment_ContentTypeM4S(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := &models.Library{Path: "/l-seg-content", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := &models.MediaItem{
		LibraryID: lib.ID, Title: "Film", FilePath: "/l-seg-content/f.mkv",
		MediaType: models.MediaTypeMovie, TranscodeStatus: models.TranscodeStatusDone,
	}
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	segDir := t.TempDir()
	segFile := filepath.Join(segDir, "seg-1.m4s")
	if err := os.MkdirAll(filepath.Dir(segFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(segFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	mpdPath := filepath.Join(segDir, "manifest.mpd")
	if err := os.WriteFile(mpdPath, []byte("<MPD/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.SetMediaMPDPath(ctx, db, m.ID, mpdPath); err != nil {
		t.Fatal(err)
	}

	cache := &dash.Cache{}
	h := handler.NewStreamHandler(db, cache, testSecret)
	r := newStreamRouter(t, h)

	u := createUser(t, db, "user5", "u5@x.com", "pass", false)
	tok := bearerToken(t, u.ID, false)

	rec := do(t, r, http.MethodGet,
		"/api/v1/stream/"+m.ID.String()+"/segments/seg-1.m4s",
		nil, map[string]string{"Authorization": "Bearer " + tok})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "video/iso.segment" {
		t.Errorf("Content-Type = %q, want video/iso.segment", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want immutable", cc)
	}
}

// --- HistoryHandler tests ---

func TestGetHistory_Empty(t *testing.T) {
	db := openTestDB(t)
	h := handler.NewHistoryHandler(db)
	r := newHistoryRouter(t, h)

	u := createUser(t, db, "histuser1", "h1@x.com", "pass", false)
	tok := bearerToken(t, u.ID, false)

	rec := do(t, r, http.MethodGet, "/api/v1/history", nil,
		map[string]string{"Authorization": "Bearer " + tok})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var items []map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&items)
	if len(items) != 0 {
		t.Errorf("expected empty list, got %d items", len(items))
	}
}

func TestUpsertHistory_MediaNotFound(t *testing.T) {
	db := openTestDB(t)
	h := handler.NewHistoryHandler(db)
	r := newHistoryRouter(t, h)

	u := createUser(t, db, "histuser2", "h2@x.com", "pass", false)
	tok := bearerToken(t, u.ID, false)

	rec := do(t, r, http.MethodPut,
		"/api/v1/history/00000000-0000-0000-0000-000000000099",
		jsonBody(map[string]interface{}{"position": 10.0, "completed": false}),
		map[string]string{"Authorization": "Bearer " + tok})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestUpsertHistory_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := &models.MediaItem{
		LibraryID: lib.ID, Title: "TestFilm", FilePath: "/l/tf.mkv",
		MediaType: models.MediaTypeMovie, TranscodeStatus: models.TranscodeStatusPending,
	}
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	h := handler.NewHistoryHandler(db)
	r := newHistoryRouter(t, h)

	u := createUser(t, db, "histuser3", "h3@x.com", "pass", false)
	tok := bearerToken(t, u.ID, false)

	// Upsert position.
	rec := do(t, r, http.MethodPut,
		"/api/v1/history/"+m.ID.String(),
		jsonBody(map[string]interface{}{"position": 55.5, "completed": false}),
		map[string]string{"Authorization": "Bearer " + tok})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	// GET history — should include it.
	rec = do(t, r, http.MethodGet, "/api/v1/history", nil,
		map[string]string{"Authorization": "Bearer " + tok})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /history status = %d", rec.Code)
	}
	var items []map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if pos, ok := items[0]["position"].(float64); !ok || pos != 55.5 {
		t.Errorf("position = %v, want 55.5", items[0]["position"])
	}
}

func TestUpsertHistory_CompletedExcludedFromList(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := &models.MediaItem{
		LibraryID: lib.ID, Title: "Done", FilePath: "/l/done.mkv",
		MediaType: models.MediaTypeMovie, TranscodeStatus: models.TranscodeStatusPending,
	}
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	h := handler.NewHistoryHandler(db)
	r := newHistoryRouter(t, h)

	u := createUser(t, db, "histuser4", "h4@x.com", "pass", false)
	tok := bearerToken(t, u.ID, false)

	// Mark as completed.
	rec := do(t, r, http.MethodPut,
		"/api/v1/history/"+m.ID.String(),
		jsonBody(map[string]interface{}{"position": 5400.0, "completed": true}),
		map[string]string{"Authorization": "Bearer " + tok})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// GET history — should be empty (completed filtered out).
	rec = do(t, r, http.MethodGet, "/api/v1/history", nil,
		map[string]string{"Authorization": "Bearer " + tok})
	var items []map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&items)
	if len(items) != 0 {
		t.Errorf("expected 0 items (completed filtered), got %d", len(items))
	}
}

func TestServeSegment_NotFound(t *testing.T) {
	db := openTestDB(t)
	cache := &dash.Cache{}
	h := handler.NewStreamHandler(db, cache, testSecret)
	r := newStreamRouter(t, h)

	u := createUser(t, db, "user6", "u6@x.com", "pass", false)
	tok := bearerToken(t, u.ID, false)

	rec := do(t, r, http.MethodGet,
		"/api/v1/stream/00000000-0000-0000-0000-000000000002/segments/notexist.m4s",
		nil, map[string]string{"Authorization": "Bearer " + tok})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// Ensure newStreamRouter/newHistoryRouter are not flagged as unused.
var _ = routerWithStream
var _ = newStreamRouter
var _ = newHistoryRouter

func init() {
	// Use httptest.NewRecorder to confirm import is used.
	_ = httptest.NewRecorder
}
