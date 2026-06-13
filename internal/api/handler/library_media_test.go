package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/ringmaster217/prism/internal/api/handler"
	apimw "github.com/ringmaster217/prism/internal/api/middleware"
	"github.com/ringmaster217/prism/internal/models"
	"github.com/ringmaster217/prism/internal/scanner"
	"github.com/ringmaster217/prism/internal/store/sqlite"
)

// newTestRouterPhase2 extends the Phase 1 test router with Phase 2 routes.
func newTestRouterPhase2(t *testing.T) (http.Handler, func()) {
	t.Helper()
	db := openTestDB(t)
	mgr := scanner.NewManager(db, nil, nil) // no ffprobe, enricher, or bus in tests

	authH := handler.NewAuthHandler(db, testSecret)
	userH := handler.NewUsersHandler(db, testSecret)
	libH := handler.NewLibraryHandler(db, mgr)
	mediaH := handler.NewMediaHandler(db)

	indexer := scanner.NewIndexer(db, nil)
	artifactH := handler.NewArtifactHandler(db, indexer)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)

	r.Post("/api/v1/auth/login", authH.Login)
	r.With(apimw.OptionalAuthenticate(testSecret)).Post("/api/v1/users", userH.CreateUser)

	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))

		r.Get("/api/v1/me", userH.GetMe)

		r.Get("/api/v1/libraries", libH.ListLibraries)
		r.With(apimw.RequireAdmin).Post("/api/v1/libraries", libH.CreateLibrary)
		r.Get("/api/v1/libraries/{id}", libH.GetLibrary)
		r.With(apimw.RequireAdmin).Delete("/api/v1/libraries/{id}", libH.DeleteLibrary)
		r.With(apimw.RequireAdmin).Post("/api/v1/libraries/{id}/scan", libH.ScanLibrary)

		r.Get("/api/v1/media", mediaH.ListMedia)
		r.Get("/api/v1/media/{id}", mediaH.GetMedia)
		r.With(apimw.RequireAdmin).Delete("/api/v1/media/{id}", mediaH.DeleteMedia)
		r.With(apimw.RequireAdmin).Post("/api/v1/admin/artifacts/write-sidecars", artifactH.HandleWriteSidecars)
	})

	cleanup := func() { _ = db.Close() }
	return r, cleanup
}

// setupAdmin inserts an admin user in db and returns a bearer token.
func setupAdminUser(t *testing.T, router http.Handler) string {
	t.Helper()
	rec := do(t, router, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "admin", "email": "a@x.com", "password": "pw"}),
		nil,
	)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create admin: status %d: %s", rec.Code, rec.Body)
	}

	loginRec := do(t, router, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "admin", "password": "pw"}),
		nil,
	)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login: status %d", loginRec.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(loginRec.Body).Decode(&resp)
	return resp["access_token"].(string)
}

func adminHeader(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

// ---------------------------------------------------------------------------
// Library CRUD tests
// ---------------------------------------------------------------------------

func TestCreateLibrary_Success(t *testing.T) {
	router, cleanup := newTestRouterPhase2(t)
	defer cleanup()
	token := setupAdminUser(t, router)

	rec := do(t, router, http.MethodPost, "/api/v1/libraries",
		jsonBody(map[string]any{"path": "/tmp/movies", "media_type": "movie"}),
		adminHeader(token),
	)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["id"] == "" {
		t.Error("expected id in response")
	}
	if resp["media_type"] != "movie" {
		t.Errorf("media_type = %v", resp["media_type"])
	}
}

func TestCreateLibrary_RequiresAdmin(t *testing.T) {
	router, cleanup := newTestRouterPhase2(t)
	defer cleanup()
	token := setupAdminUser(t, router)

	// Create a regular user.
	db := openTestDB(t)
	normalUser := createUser(t, db, "user", "user@x.com", "pw", false)
	normalToken := bearerToken(t, normalUser.ID, false)

	rec := do(t, router, http.MethodPost, "/api/v1/libraries",
		jsonBody(map[string]any{"path": "/tmp/x", "media_type": "movie"}),
		map[string]string{"Authorization": "Bearer " + normalToken},
	)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (token=%s, admin_token=%s)", rec.Code, normalToken, token)
	}
}

func TestCreateLibrary_InvalidMediaType(t *testing.T) {
	router, cleanup := newTestRouterPhase2(t)
	defer cleanup()
	token := setupAdminUser(t, router)

	rec := do(t, router, http.MethodPost, "/api/v1/libraries",
		jsonBody(map[string]any{"path": "/tmp/x", "media_type": "podcast"}),
		adminHeader(token),
	)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateLibrary_MissingFields(t *testing.T) {
	router, cleanup := newTestRouterPhase2(t)
	defer cleanup()
	token := setupAdminUser(t, router)

	rec := do(t, router, http.MethodPost, "/api/v1/libraries",
		jsonBody(map[string]any{"path": "/tmp/movies"}),
		adminHeader(token),
	)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestListLibraries_EmptyArray(t *testing.T) {
	router, cleanup := newTestRouterPhase2(t)
	defer cleanup()
	token := setupAdminUser(t, router)

	rec := do(t, router, http.MethodGet, "/api/v1/libraries", nil, adminHeader(token))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp []any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp) != 0 {
		t.Errorf("want [], got %v", resp)
	}
}

func TestGetLibrary_NotFound(t *testing.T) {
	router, cleanup := newTestRouterPhase2(t)
	defer cleanup()
	token := setupAdminUser(t, router)

	rec := do(t, router, http.MethodGet, "/api/v1/libraries/00000000-0000-0000-0000-000000000000",
		nil, adminHeader(token))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteLibrary_Success(t *testing.T) {
	router, cleanup := newTestRouterPhase2(t)
	defer cleanup()
	token := setupAdminUser(t, router)

	createRec := do(t, router, http.MethodPost, "/api/v1/libraries",
		jsonBody(map[string]any{"path": "/tmp/del", "media_type": "movie"}),
		adminHeader(token),
	)
	var lib map[string]any
	_ = json.NewDecoder(createRec.Body).Decode(&lib)
	libID := lib["id"].(string)

	delRec := do(t, router, http.MethodDelete, "/api/v1/libraries/"+libID, nil, adminHeader(token))
	if delRec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204; body: %s", delRec.Code, delRec.Body)
	}

	getRec := do(t, router, http.MethodGet, "/api/v1/libraries/"+libID, nil, adminHeader(token))
	if getRec.Code != http.StatusNotFound {
		t.Errorf("after delete, GET status = %d, want 404", getRec.Code)
	}
}

func TestScanLibrary_Accepted(t *testing.T) {
	router, cleanup := newTestRouterPhase2(t)
	defer cleanup()
	token := setupAdminUser(t, router)

	createRec := do(t, router, http.MethodPost, "/api/v1/libraries",
		jsonBody(map[string]any{"path": t.TempDir(), "media_type": "movie"}),
		adminHeader(token),
	)
	var lib map[string]any
	_ = json.NewDecoder(createRec.Body).Decode(&lib)

	scanRec := do(t, router, http.MethodPost, "/api/v1/libraries/"+lib["id"].(string)+"/scan",
		nil, adminHeader(token))
	if scanRec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", scanRec.Code)
	}
}

// ---------------------------------------------------------------------------
// Media item tests
// ---------------------------------------------------------------------------

func TestListMedia_EmptyArray(t *testing.T) {
	router, cleanup := newTestRouterPhase2(t)
	defer cleanup()
	token := setupAdminUser(t, router)

	rec := do(t, router, http.MethodGet, "/api/v1/media", nil, adminHeader(token))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp []any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp) != 0 {
		t.Errorf("want [], got %v", resp)
	}
}

func TestListMedia_IncludesEpisodesWithAll(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeTVShow}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}

	movie := &models.MediaItem{
		LibraryID: lib.ID, Title: "Movie Title", MediaType: models.MediaTypeMovie,
		FilePath: "/l/movie.mkv", TranscodeStatus: models.TranscodeStatusPending,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, movie); err != nil {
		t.Fatal(err)
	}

	episode := &models.MediaItem{
		LibraryID: lib.ID, Title: "Episode Title", MediaType: models.MediaTypeEpisode,
		FilePath: "/l/episode.mkv", TranscodeStatus: models.TranscodeStatusPending,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, episode); err != nil {
		t.Fatal(err)
	}

	mediaH := handler.NewMediaHandler(db)
	r := chi.NewRouter()
	r.Use(apimw.Authenticate(testSecret))
	r.Get("/api/v1/media", mediaH.ListMedia)

	adminUser := createUser(t, db, "adm", "adm@x.com", "pw", true)
	hdr := map[string]string{"Authorization": "Bearer " + bearerToken(t, adminUser.ID, true)}

	// 1. Without all=true, episode should be excluded
	rec1 := do(t, r, http.MethodGet, "/api/v1/media", nil, hdr)
	if rec1.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec1.Code)
	}
	var resp1 []models.MediaItem
	_ = json.NewDecoder(rec1.Body).Decode(&resp1)
	if len(resp1) != 1 {
		t.Errorf("expected 1 media item (movie), got %d", len(resp1))
	} else if resp1[0].Title != "Movie Title" {
		t.Errorf("expected Movie Title, got %s", resp1[0].Title)
	}

	// 2. With all=true, episode should be included
	rec2 := do(t, r, http.MethodGet, "/api/v1/media?all=true", nil, hdr)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec2.Code)
	}
	var resp2 []models.MediaItem
	_ = json.NewDecoder(rec2.Body).Decode(&resp2)
	if len(resp2) != 2 {
		t.Errorf("expected 2 media items (movie + episode), got %d", len(resp2))
	}
}

func TestGetMedia_NotFound(t *testing.T) {
	router, cleanup := newTestRouterPhase2(t)
	defer cleanup()
	token := setupAdminUser(t, router)

	rec := do(t, router, http.MethodGet, "/api/v1/media/00000000-0000-0000-0000-000000000000",
		nil, adminHeader(token))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteMedia_Success(t *testing.T) {
	router, cleanup := newTestRouterPhase2(t)
	defer cleanup()
	token := setupAdminUser(t, router)

	// Insert a library + media item directly via the store.
	db := openTestDB(t)
	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m := &models.MediaItem{
		LibraryID: lib.ID, Title: "X", MediaType: models.MediaTypeMovie,
		FilePath: "/l/x.mkv", TranscodeStatus: models.TranscodeStatusPending,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}

	// Build a minimal router backed by the same db.
	mediaH2 := handler.NewMediaHandler(db)
	r2 := chi.NewRouter()
	r2.Use(apimw.Authenticate(testSecret))
	r2.With(apimw.RequireAdmin).Delete("/api/v1/media/{id}", mediaH2.DeleteMedia)
	r2.Get("/api/v1/media/{id}", mediaH2.GetMedia)

	adminUser := createUser(t, db, "adm", "adm@x.com", "pw", true)
	hdr := map[string]string{"Authorization": "Bearer " + bearerToken(t, adminUser.ID, true)}
	_ = token // suppress unused warning (token is from the outer router's DB)

	delRec := do(t, r2, http.MethodDelete, "/api/v1/media/"+m.ID.String(), nil, hdr)
	if delRec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204; body: %s", delRec.Code, delRec.Body)
	}

	getRec := do(t, r2, http.MethodGet, "/api/v1/media/"+m.ID.String(), nil, hdr)
	if getRec.Code != http.StatusNotFound {
		t.Errorf("after delete, GET status = %d, want 404", getRec.Code)
	}
}

func TestWriteSidecars_Endpoint(t *testing.T) {
	router, cleanup := newTestRouterPhase2(t)
	defer cleanup()
	token := setupAdminUser(t, router)

	// Re-route check
	recNotFound := do(t, router, http.MethodPost, "/api/v1/admin/artifacts/write-sidecars", nil, adminHeader(token))
	if recNotFound.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", recNotFound.Code, recNotFound.Body)
	}

	// We'll test with a real DB and some items to see bulk behavior
	db := openTestDB(t)
	lib := &models.Library{Path: t.TempDir(), MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}

	// Media 1: No transcode bundle (mpd_path is null/empty) -> skipped
	m1 := &models.MediaItem{
		LibraryID: lib.ID, Title: "M1", MediaType: models.MediaTypeMovie,
		FilePath: filepath.Join(lib.Path, "m1.mp4"), TranscodeStatus: models.TranscodeStatusPending,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, m1); err != nil {
		t.Fatal(err)
	}

	// Media 2: Has transcode bundle, file exists -> written successfully
	m2 := &models.MediaItem{
		LibraryID: lib.ID, Title: "M2", MediaType: models.MediaTypeMovie,
		FilePath: filepath.Join(lib.Path, "m2.mp4"), TranscodeStatus: models.TranscodeStatusDone,
	}
	outputDir := t.TempDir()
	mpdPath := filepath.Join(outputDir, "manifest.mpd")
	m2.MPDPath = &mpdPath
	if err := sqlite.UpsertMediaItem(context.Background(), db, m2); err != nil {
		t.Fatal(err)
	}

	// Make sure the file exists for the fingerprinting step
	if err := os.WriteFile(m2.FilePath, []byte("m2 test data"), 0644); err != nil {
		t.Fatal(err)
	}

	indexer := scanner.NewIndexer(db, nil)
	artifactH := handler.NewArtifactHandler(db, indexer)

	r2 := chi.NewRouter()
	r2.Use(apimw.Authenticate(testSecret))
	r2.With(apimw.RequireAdmin).Post("/api/v1/admin/artifacts/write-sidecars", artifactH.HandleWriteSidecars)

	adminUser := createUser(t, db, "adm", "adm@x.com", "pw", true)
	hdr := map[string]string{"Authorization": "Bearer " + bearerToken(t, adminUser.ID, true)}

	rec := do(t, r2, http.MethodPost, "/api/v1/admin/artifacts/write-sidecars", nil, hdr)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d: %s", rec.Code, rec.Body)
	}

	var resp map[string]int
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["written"] != 1 {
		t.Errorf("expected written = 1, got %d", resp["written"])
	}
	if resp["skipped"] != 1 {
		t.Errorf("expected skipped = 1, got %d", resp["skipped"])
	}
	if resp["errors"] != 0 {
		t.Errorf("expected errors = 0, got %d", resp["errors"])
	}
}

