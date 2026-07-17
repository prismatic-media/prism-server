package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/migrations"
)

func openEnricherTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("goose.Up: %v", err)
	}
	return db
}

// setTMDBKey seeds the tmdb_api_key setting in the test DB.
func setTMDBKey(t *testing.T, db *sql.DB, key string) {
	t.Helper()
	if err := sqlite.SetSetting(context.Background(), db, "tmdb_api_key", key); err != nil {
		t.Fatalf("setTMDBKey: %v", err)
	}
}

// seedItem inserts a library + media item and returns the item.
func seedItem(t *testing.T, db *sql.DB, mediaType models.MediaType, filePath string) *models.MediaItem {
	t.Helper()
	lib := &models.Library{
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
	// No tmdb_api_key set in DB — enricher should be a no-op.
	e := NewEnricher(db)
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

	setTMDBKey(t, db, "test-key")
	e := NewEnricher(db)
	e.baseURLOverride = srv.URL

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

	movieDetailResp, _ := json.Marshal(map[string]any{
		"id":           float64(27205),
		"title":        "Inception",
		"release_date": "2010-07-16",
		"overview":     "A thief who steals corporate secrets.",
		"poster_path":  "",
		"credits": map[string]any{
			"cast": []any{
				map[string]any{"name": "Leonardo DiCaprio", "character": "Cobb"},
			},
			"crew": []any{
				map[string]any{"name": "Christopher Nolan", "job": "Director"},
			},
		},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/movie":
			_, _ = w.Write(movieResp)
		case "/movie/27205":
			_, _ = w.Write(movieDetailResp)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	item := seedItem(t, db, models.MediaTypeMovie, "/movies/Inception (2010).mkv")

	setTMDBKey(t, db, "test-key")
	e := NewEnricher(db)
	e.baseURLOverride = srv.URL

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

	tvDetailResp, _ := json.Marshal(map[string]any{
		"id":             float64(1396),
		"name":           "Breaking Bad",
		"first_air_date": "2008-01-20",
		"overview":       "A teacher turns cook.",
		"poster_path":    "",
		"created_by": []any{
			map[string]any{"name": "Vince Gilligan"},
		},
		"credits": map[string]any{
			"cast": []any{
				map[string]any{"name": "Bryan Cranston", "character": "Walter White"},
			},
		},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/tv":
			_, _ = w.Write(tvResp)
		case "/tv/1396":
			_, _ = w.Write(tvDetailResp)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	item := seedItem(t, db, models.MediaTypeTVShow, "/tv/Breaking.Bad.2008.mkv")

	setTMDBKey(t, db, "test-key")
	e := NewEnricher(db)
	e.baseURLOverride = srv.URL

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
		_, _ = w.Write(b)
	}))
	defer emptySrv.Close()

	item := seedItem(t, db, models.MediaTypeMovie, "/movies/UnknownFilm.mkv")

	setTMDBKey(t, db, "test-key")
	e := NewEnricher(db)
	e.baseURLOverride = emptySrv.URL

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

	setTMDBKey(t, db, "test-key")
	e := NewEnricher(db)
	e.baseURLOverride = srv.URL

	e.EnrichItem(context.Background(), item)
}

func TestEnricher_Movie_YearFallback(t *testing.T) {
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

	var searchCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/search/movie" {
			searchCount++
			q := r.URL.Query()
			// First call has year filter, we return empty to trigger fallback
			if q.Get("year") == "2009" {
				_, _ = w.Write(tmdbEmptyResponse())
			} else {
				_, _ = w.Write(movieResp)
			}
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	item := seedItem(t, db, models.MediaTypeMovie, "/movies/Inception (2009).mkv")

	setTMDBKey(t, db, "test-key")
	e := NewEnricher(db)
	e.baseURLOverride = srv.URL

	e.EnrichItem(context.Background(), item)

	if searchCount != 2 {
		t.Errorf("expected 2 searches (one with year=2009, one fallback), got %d", searchCount)
	}

	updated, err := sqlite.GetMediaItemByID(context.Background(), db, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.TMDBId == nil || *updated.TMDBId != 27205 {
		t.Errorf("expected movie to be matched via fallback, got tmdb_id = %v", updated.TMDBId)
	}
}

func TestEnricher_Movie_TitleOverwrite(t *testing.T) {
	db := openEnricherTestDB(t)

	movieResp, _ := json.Marshal(map[string]any{
		"results": []map[string]any{{
			"id":           float64(27205),
			"title":        "Inception (Official TMDB Title)",
			"release_date": "2010-07-16",
			"overview":     "Overview...",
			"poster_path":  "",
		}},
	})

	movieDetailResp, _ := json.Marshal(map[string]any{
		"id":           float64(27205),
		"title":        "Inception (Official TMDB Title)",
		"release_date": "2010-07-16",
		"overview":     "Overview...",
		"poster_path":  "",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/search/movie" {
			_, _ = w.Write(movieResp)
		} else if r.URL.Path == "/movie/27205" {
			_, _ = w.Write(movieDetailResp)
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	item := seedItem(t, db, models.MediaTypeMovie, "/movies/Inception.mkv")

	setTMDBKey(t, db, "test-key")
	e := NewEnricher(db)
	e.baseURLOverride = srv.URL

	e.EnrichItem(context.Background(), item)

	updated, err := sqlite.GetMediaItemByID(context.Background(), db, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Inception (Official TMDB Title)" {
		t.Errorf("Title: got %q, want %q", updated.Title, "Inception (Official TMDB Title)")
	}
}

func TestEnricher_TVShow_Merging(t *testing.T) {
	db := openEnricherTestDB(t)

	tvResp, _ := json.Marshal(map[string]any{
		"results": []map[string]any{{
			"id":             float64(1668),
			"name":           "Bluey",
			"first_air_date": "2018-10-01",
			"overview":       "Overview...",
			"poster_path":    "",
		}},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(tvResp)
	}))
	defer srv.Close()

	lib := &models.Library{
		Path:      "/tvshows",
		MediaType: models.MediaTypeTVShow,
	}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}

	show1 := &models.TVShow{
		LibraryID: lib.ID,
		Name:      "Bluey",
	}
	if err := sqlite.UpsertTVShow(context.Background(), db, show1); err != nil {
		t.Fatal(err)
	}

	show2 := &models.TVShow{
		LibraryID: lib.ID,
		Name:      "Bluey (2018)",
	}
	if err := sqlite.UpsertTVShow(context.Background(), db, show2); err != nil {
		t.Fatal(err)
	}

	season := &models.TVSeason{
		TVShowID:     show2.ID,
		SeasonNumber: 1,
	}
	if err := sqlite.UpsertTVSeason(context.Background(), db, season); err != nil {
		t.Fatal(err)
	}

	episode := &models.MediaItem{
		LibraryID:       lib.ID,
		Title:           "Episode 1",
		MediaType:       models.MediaTypeEpisode,
		FilePath:        "/tvshows/Bluey (2018)/Season 1/Bluey (2018) - s01e01.mp4",
		FileSize:        1024,
		TVShowID:        &show2.ID,
		TVSeasonID:      &season.ID,
		SeasonNumber:    &season.SeasonNumber,
		EpisodeNumber:   &season.SeasonNumber,
		TranscodeStatus: models.TranscodeStatusNone,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, episode); err != nil {
		t.Fatal(err)
	}

	setTMDBKey(t, db, "test-key")
	e := NewEnricher(db)
	e.baseURLOverride = srv.URL

	// Enriching show2 should trigger merge into show1 because they both resolve to name "Bluey"
	e.EnrichTVShow(context.Background(), show2)

	// show2 content should be overwritten with canonical show1 details
	if show2.ID != show1.ID {
		t.Errorf("expected show2 ID to be updated to canonical ID %v, got %v", show1.ID, show2.ID)
	}

	// Duplicate show should be removed
	_, err := sqlite.GetTVShowByID(context.Background(), db, show2.ID)
	if err != nil {
		t.Fatalf("canonical show should exist, but got error: %v", err)
	}

	// Verify that show2 (duplicate) is deleted or merged
	shows, err := sqlite.ListTVShows(context.Background(), db, lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(shows) != 1 {
		t.Errorf("expected 1 show, got %d", len(shows))
	}

	// Verify that the episode's tv_show_id was updated to show1.ID
	ep, err := sqlite.GetMediaItemByID(context.Background(), db, episode.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ep.TVShowID == nil || *ep.TVShowID != show1.ID {
		t.Errorf("expected episode's TVShowID to be updated to %v, got %v", show1.ID, ep.TVShowID)
	}
}

func TestEnricher_MovieScoringAndMatching(t *testing.T) {
	db := openEnricherTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "/search/movie") {
			resp := map[string]any{
				"results": []map[string]any{
					{
						"id":           100,
						"title":        "Jurassic World",
						"release_date": "2015-06-12",
						"popularity":   85.5,
						"overview":     "Jurassic World main movie",
						"poster_path":  "/jw.jpg",
					},
					{
						"id":           200,
						"title":        "Jurassic World Rebirth",
						"release_date": "2025-07-02",
						"popularity":   50.2,
						"overview":     "Rebirth sequel",
						"poster_path":  "/jwr.jpg",
					},
				},
			}
			b, _ := json.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		if strings.Contains(r.URL.Path, "/movie/100") {
			resp := map[string]any{
				"id":           100,
				"title":        "Jurassic World",
				"release_date": "2015-06-12",
				"runtime":      124,
				"overview":     "Jurassic World main movie",
				"poster_path":  "/jw.jpg",
			}
			b, _ := json.Marshal(resp)
			_, _ = w.Write(b)
			return
		}
		if strings.Contains(r.URL.Path, "/movie/200") {
			resp := map[string]any{
				"id":           200,
				"title":        "Jurassic World Rebirth",
				"release_date": "2025-07-02",
				"runtime":      110,
				"overview":     "Rebirth sequel",
				"poster_path":  "/jwr.jpg",
			}
			b, _ := json.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	setTMDBKey(t, db, "test-key")
	e := NewEnricher(db)
	e.baseURLOverride = srv.URL

	item1 := seedItem(t, db, models.MediaTypeMovie, "/movies/Jurassic World (2015).mkv")
	item1.Duration = 7440.0 // 124 minutes

	e.EnrichItem(context.Background(), item1)

	updated1, err := sqlite.GetMediaItemByID(context.Background(), db, item1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated1.TMDBId == nil || *updated1.TMDBId != 100 {
		t.Errorf("expected match ID 100, got %v", updated1.TMDBId)
	}
	if updated1.Title != "Jurassic World" {
		t.Errorf("expected Title 'Jurassic World', got %q", updated1.Title)
	}
}

func TestEnricher_MovieLowConfidence_Rejected(t *testing.T) {
	db := openEnricherTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/search/movie") {
			resp := map[string]any{
				"results": []map[string]any{
					{
						"id":           999,
						"title":        "Interstellar",
						"release_date": "2014-11-07",
						"popularity":   10.0,
					},
				},
			}
			b, _ := json.Marshal(resp)
			_, _ = w.Write(b)
			return
		}
		if strings.Contains(r.URL.Path, "/movie/999") {
			resp := map[string]any{
				"id":      999,
				"title":   "Interstellar",
				"runtime": 169,
			}
			b, _ := json.Marshal(resp)
			_, _ = w.Write(b)
			return
		}
	}))
	defer srv.Close()

	setTMDBKey(t, db, "test-key")
	e := NewEnricher(db)
	e.baseURLOverride = srv.URL

	item := seedItem(t, db, models.MediaTypeMovie, "/movies/Jurassic World (2015).mkv")
	item.Duration = 7440.0

	e.EnrichItem(context.Background(), item)

	updated, err := sqlite.GetMediaItemByID(context.Background(), db, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.TMDBId != nil {
		t.Errorf("expected TMDB ID to be nil (rejected match), got %d", *updated.TMDBId)
	}
}
