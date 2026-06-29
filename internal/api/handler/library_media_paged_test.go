package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/prismatic-media/prism-server/internal/api/handler"
	apimw "github.com/prismatic-media/prism-server/internal/api/middleware"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

func TestListMedia_Pagination(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	mediaH := handler.NewMediaHandler(db)
	r := chi.NewRouter()
	r.Use(apimw.Authenticate(testSecret))
	r.Get("/api/v1/movies", mediaH.ListMedia)

	adminUser := createUser(t, db, "adm", "adm@x.com", "pw", true)
	hdr := map[string]string{"Authorization": "Bearer " + bearerToken(t, adminUser.ID, true)}

	// Create library
	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}

	// Insert 5 test movies
	for i := 1; i <= 5; i++ {
		m := &models.MediaItem{
			LibraryID: lib.ID,
			Title:     fmt.Sprintf("Movie %d", i),
			MediaType: models.MediaTypeMovie,
			FilePath:  fmt.Sprintf("/l/movie%d.mkv", i),
			TranscodeStatus: models.TranscodeStatusPending,
		}
		if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("Standard List returns array (backward compatibility)", func(t *testing.T) {
		rec := do(t, r, http.MethodGet, "/api/v1/movies", nil, hdr)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var resp []models.MediaItem
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}
		if len(resp) != 5 {
			t.Errorf("expected 5 items, got %d", len(resp))
		}
	})

	t.Run("Paginated List with page_size = 2", func(t *testing.T) {
		rec := do(t, r, http.MethodGet, "/api/v1/movies?page_size=2", nil, hdr)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp handler.PaginatedMoviesResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}

		if len(resp.Movies) != 2 {
			t.Fatalf("expected 2 movies in page 1, got %d", len(resp.Movies))
		}
		if resp.Movies[0].Title != "Movie 1" || resp.Movies[1].Title != "Movie 2" {
			t.Errorf("unexpected page 1 items: %s, %s", resp.Movies[0].Title, resp.Movies[1].Title)
		}
		if resp.NextPageToken == "" {
			t.Fatal("expected page token, got empty")
		}

		// Fetch page 2
		url2 := fmt.Sprintf("/api/v1/movies?page_size=2&page_token=%s", resp.NextPageToken)
		rec2 := do(t, r, http.MethodGet, url2, nil, hdr)
		if rec2.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec2.Code)
		}

		var resp2 handler.PaginatedMoviesResponse
		if err := json.NewDecoder(rec2.Body).Decode(&resp2); err != nil {
			t.Fatal(err)
		}

		if len(resp2.Movies) != 2 {
			t.Fatalf("expected 2 movies in page 2, got %d", len(resp2.Movies))
		}
		if resp2.Movies[0].Title != "Movie 3" || resp2.Movies[1].Title != "Movie 4" {
			t.Errorf("unexpected page 2 items: %s, %s", resp2.Movies[0].Title, resp2.Movies[1].Title)
		}
		if resp2.NextPageToken == "" {
			t.Fatal("expected page token for page 2, got empty")
		}

		// Fetch page 3 (remaining 1 item)
		url3 := fmt.Sprintf("/api/v1/movies?page_size=2&page_token=%s", resp2.NextPageToken)
		rec3 := do(t, r, http.MethodGet, url3, nil, hdr)
		if rec3.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec3.Code)
		}

		var resp3 handler.PaginatedMoviesResponse
		if err := json.NewDecoder(rec3.Body).Decode(&resp3); err != nil {
			t.Fatal(err)
		}

		if len(resp3.Movies) != 1 {
			t.Fatalf("expected 1 movie in page 3, got %d", len(resp3.Movies))
		}
		if resp3.Movies[0].Title != "Movie 5" {
			t.Errorf("unexpected page 3 item: %s", resp3.Movies[0].Title)
		}
		if resp3.NextPageToken != "" {
			t.Errorf("expected empty next page token for last page, got %q", resp3.NextPageToken)
		}
	})
}
