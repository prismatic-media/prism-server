package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/ringmaster217/galactic-media-server/internal/models"
	"github.com/ringmaster217/galactic-media-server/internal/store/sqlite"
	"github.com/ringmaster217/galactic-media-server/migrations"
)

func openEnricherTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("goose.Up: %v", err)
	}
	return db
}

// seedItem inserts a library + media item and returns the item.
func seedItem(t *testing.T, db *sql.DB, mediaType models.MediaType, filePath string) *models.MediaItem {
	t.Helper()
	lib := &models.Library{
		Name:      "Test",
		Path:      "/test",
		MediaType: mediaType,
	}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	item := &models.MediaItem{
		LibraryID:       lib.ID,
		Title:           "Test Item",
		MediaType:       mediaType,
		FilePath:        filePath,
		FileSize:        1024,
		TranscodeStatus: models.TranscodeStatusPending,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, item); err != nil {
		t.Fatal(err)
	}
	// Fetch to get the canonical ID from the DB.
	fetched, err := sqlite.GetMediaItemByPath(context.Background(), db, filePath)
	if err != nil {
		t.Fatal(err)
	}
	return fetched
}

func TestEnricher_NoAPIKey_IsNoop(t *testing.T) {
	db := openEnricherTestDB(t)
	e := NewEnricher(db, "", "")
	item := &models.MediaItem{
		ID:        uuid.New(),
		MediaType: models.MediaTypeMovie,
		FilePath:  "/movies/Inception (2010).mkv",
	}
	// Should not panic or error.
	e.EnrichItem(context.Background(), item)
}

func TestEnricher_AlreadyEnriched_IsNoop(t *testing.T) {
	db := openEnricherTestDB(t)
	// Set up a fake TMDB server that would fail if called.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("TMDB should not be called for already-enriched item")
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	e := NewEnricher(db, "test-key", "")
	e.client.baseURL = srv.URL

	tmdbID := 27205
	item := &models.MediaItem{
		ID:        uuid.New(),
		MediaType: models.MediaTypeMovie,
		FilePath:  "/movies/Inception (2010).mkv",
		TMDBId:    &tmdbID,
	}
	e.EnrichItem(context.Background(), item)
}

func TestEnricher_Movie_WritesMetadata(t *testing.T) {
	db := openEnricherTestDB(t)

	movieResp, _ := json.Marshal(map[string]any{
		"results": []map[string]any{{
			"id":           float64(27205),
			"title":        "Inception",
			"release_date": "2010-07-16",
			"overview":     "A thief who steals corporate secrets.",
			"poster_path":  "",
		}},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(movieResp)
	}))
	defer srv.Close()

	item := seedItem(t, db, models.MediaTypeMovie, "/movies/Inception (2010).mkv")

	e := NewEnricher(db, "test-key", "")
	e.client.baseURL = srv.URL

	e.EnrichItem(context.Background(), item)

	updated, err := sqlite.GetMediaItemByID(context.Background(), db, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.TMDBId == nil {
		t.Fatal("expected TMDB ID to be set")
	}
	if *updated.TMDBId != 27205 {
		t.Errorf("TMDBId: got %d, want 27205", *updated.TMDBId)
	}
	if updated.Year == nil || *updated.Year != 2010 {
		t.Errorf("Year: got %v, want 2010", updated.Year)
	}
	if updated.Overview == nil || *updated.Overview == "" {
		t.Error("expected overview to be set")
	}
}

func TestEnricher_TVShow_WritesMetadata(t *testing.T) {
	db := openEnricherTestDB(t)

	tvResp, _ := json.Marshal(map[string]any{
		"results": []map[string]any{{
			"id":             float64(1396),
			"name":           "Breaking Bad",
			"first_air_date": "2008-01-20",
			"overview":       "A teacher turns cook.",
			"poster_path":    "",
		}},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(tvResp)
	}))
	defer srv.Close()

	item := seedItem(t, db, models.MediaTypeTVShow, "/tv/Breaking.Bad.2008.mkv")

	e := NewEnricher(db, "test-key", "")
	e.client.baseURL = srv.URL

	e.EnrichItem(context.Background(), item)

	updated, err := sqlite.GetMediaItemByID(context.Background(), db, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.TMDBId == nil || *updated.TMDBId != 1396 {
		t.Errorf("TMDBId: got %v, want 1396", updated.TMDBId)
	}
}

func TestEnricher_NoResults_NoUpdate(t *testing.T) {
	db := openEnricherTestDB(t)

	emptySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		b, _ := json.Marshal(map[string]any{"results": []any{}})
		w.Write(b)
	}))
	defer emptySrv.Close()

	item := seedItem(t, db, models.MediaTypeMovie, "/movies/UnknownFilm.mkv")

	e := NewEnricher(db, "test-key", "")
	e.client.baseURL = emptySrv.URL

	e.EnrichItem(context.Background(), item)

	updated, err := sqlite.GetMediaItemByID(context.Background(), db, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.TMDBId != nil {
		t.Errorf("expected TMDBId to remain nil, got %d", *updated.TMDBId)
	}
}

func TestEnricher_MusicType_IsNoop(t *testing.T) {
	db := openEnricherTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("TMDB should not be called for music items")
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	item := seedItem(t, db, models.MediaTypeMusic, "/music/Song.mp3")
	// Override MediaType since seedItem uses MediaTypeMusic but the DB stores
	// what we pass in. We just need the enricher to see MediaTypeMusic.
	item.MediaType = models.MediaTypeMusic

	e := NewEnricher(db, "test-key", "")
	e.client.baseURL = srv.URL

	e.EnrichItem(context.Background(), item)
}
