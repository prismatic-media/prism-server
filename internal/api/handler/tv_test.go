package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prismatic-media/prism-server/internal/api/handler"
	apimw "github.com/prismatic-media/prism-server/internal/api/middleware"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

func TestListShows_Pagination(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	tvH := handler.NewTVHandler(db)
	r := chi.NewRouter()
	r.Use(apimw.Authenticate(testSecret))
	r.Get("/api/v1/tv-shows", tvH.ListShows)

	adminUser := createUser(t, db, "adm_tv", "adm_tv@x.com", "pw", true)
	hdr := map[string]string{"Authorization": "Bearer " + bearerToken(t, adminUser.ID, true)}

	// Create library
	lib := &models.Library{Path: "/l_tv", MediaType: models.MediaTypeTVShow}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}

	// Insert 5 test TV shows
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("TV Show %d", i)
		show := &models.TVShow{
			LibraryID: lib.ID,
			Name:      name,
		}
		if err := sqlite.UpsertTVShow(context.Background(), db, show); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("Standard List returns array (backward compatibility)", func(t *testing.T) {
		rec := do(t, r, http.MethodGet, "/api/v1/tv-shows", nil, hdr)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var resp []models.TVShow
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}
		if len(resp) != 5 {
			t.Errorf("expected 5 items, got %d", len(resp))
		}
	})

	t.Run("Paginated List with page_size = 2", func(t *testing.T) {
		rec := do(t, r, http.MethodGet, "/api/v1/tv-shows?page_size=2", nil, hdr)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp handler.PaginatedTVShowsResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}

		if len(resp.TVShows) != 2 {
			t.Fatalf("expected 2 tv shows in page 1, got %d", len(resp.TVShows))
		}
		if resp.TVShows[0].Name != "TV Show 1" || resp.TVShows[1].Name != "TV Show 2" {
			t.Errorf("unexpected page 1 items: %s, %s", resp.TVShows[0].Name, resp.TVShows[1].Name)
		}
		if resp.NextPageToken == "" {
			t.Fatal("expected page token, got empty")
		}

		// Fetch page 2
		url2 := fmt.Sprintf("/api/v1/tv-shows?page_size=2&page_token=%s", resp.NextPageToken)
		rec2 := do(t, r, http.MethodGet, url2, nil, hdr)
		if rec2.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec2.Code)
		}

		var resp2 handler.PaginatedTVShowsResponse
		if err := json.NewDecoder(rec2.Body).Decode(&resp2); err != nil {
			t.Fatal(err)
		}

		if len(resp2.TVShows) != 2 {
			t.Fatalf("expected 2 tv shows in page 2, got %d", len(resp2.TVShows))
		}
		if resp2.TVShows[0].Name != "TV Show 3" || resp2.TVShows[1].Name != "TV Show 4" {
			t.Errorf("unexpected page 2 items: %s, %s", resp2.TVShows[0].Name, resp2.TVShows[1].Name)
		}
	})
}
