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

func TestEpisodes_Routes(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	tvH := handler.NewTVHandler(db)
	r := chi.NewRouter()
	r.Use(apimw.Authenticate(testSecret))
	r.Get("/api/v1/tv-shows/{id}/seasons/{number}/episodes", tvH.ListEpisodes)
	r.Get("/api/v1/tv-shows/{id}/seasons/{number}/episodes/{episode_id}", tvH.GetEpisode)
	r.Get("/api/v1/tv-shows/{id}/seasons/{number}/episodes/{episode_id}/next", tvH.GetNextEpisode)
	r.With(apimw.RequireAdmin).Delete("/api/v1/tv-shows/{id}/seasons/{number}/episodes/{episode_id}", tvH.DeleteEpisode)
	r.Get("/api/v1/episodes", tvH.ListAllEpisodes)
	r.Get("/api/v1/episodes/{episode_id}", tvH.GetEpisodeByID)
	r.With(apimw.RequireAdmin).Delete("/api/v1/episodes/{episode_id}", tvH.DeleteEpisodeByID)

	adminUser := createUser(t, db, "adm_tv2", "adm_tv2@x.com", "pw", true)
	hdr := map[string]string{"Authorization": "Bearer " + bearerToken(t, adminUser.ID, true)}

	// Create library
	lib := &models.Library{Path: "/l_tv2", MediaType: models.MediaTypeTVShow}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}

	// Insert show & season
	show := &models.TVShow{LibraryID: lib.ID, Name: "Show X"}
	if err := sqlite.UpsertTVShow(context.Background(), db, show); err != nil {
		t.Fatal(err)
	}
	season := &models.TVSeason{TVShowID: show.ID, SeasonNumber: 1}
	if err := sqlite.UpsertTVSeason(context.Background(), db, season); err != nil {
		t.Fatal(err)
	}

	// Insert 2 test episodes
	ep1Val, ep2Val := 1, 2
	seaNum := 1
	ep1 := &models.MediaItem{
		LibraryID: lib.ID, Title: "Ep 1", MediaType: models.MediaTypeEpisode,
		FilePath: "/l_tv2/ep1.mkv", TVShowID: &show.ID, TVSeasonID: &season.ID,
		SeasonNumber: &seaNum, EpisodeNumber: &ep1Val,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, ep1); err != nil {
		t.Fatal(err)
	}
	ep2 := &models.MediaItem{
		LibraryID: lib.ID, Title: "Ep 2", MediaType: models.MediaTypeEpisode,
		FilePath: "/l_tv2/ep2.mkv", TVShowID: &show.ID, TVSeasonID: &season.ID,
		SeasonNumber: &seaNum, EpisodeNumber: &ep2Val,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, ep2); err != nil {
		t.Fatal(err)
	}

	// Test ListEpisodes
	listUrl := fmt.Sprintf("/api/v1/tv-shows/%s/seasons/1/episodes", show.ID)
	rec := do(t, r, http.MethodGet, listUrl, nil, hdr)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
	}
	var listResp []models.Episode
	if err := json.NewDecoder(rec.Body).Decode(&listResp); err != nil {
		t.Fatal(err)
	}
	if len(listResp) != 2 {
		t.Errorf("expected 2 episodes, got %d", len(listResp))
	}

	// Test GetEpisode
	getUrl := fmt.Sprintf("/api/v1/tv-shows/%s/seasons/1/episodes/%s", show.ID, ep1.ID)
	rec2 := do(t, r, http.MethodGet, getUrl, nil, hdr)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec2.Code, rec2.Body)
	}
	var epResp models.Episode
	if err := json.NewDecoder(rec2.Body).Decode(&epResp); err != nil {
		t.Fatal(err)
	}
	if epResp.Title != "Ep 1" {
		t.Errorf("expected Ep 1, got %q", epResp.Title)
	}

	// Test GetNextEpisode
	nextUrl := fmt.Sprintf("/api/v1/tv-shows/%s/seasons/1/episodes/%s/next", show.ID, ep1.ID)
	rec3 := do(t, r, http.MethodGet, nextUrl, nil, hdr)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec3.Code, rec3.Body)
	}
	var nextResp models.Episode
	if err := json.NewDecoder(rec3.Body).Decode(&nextResp); err != nil {
		t.Fatal(err)
	}
	if nextResp.Title != "Ep 2" {
		t.Errorf("expected Ep 2, got %q", nextResp.Title)
	}

	// Test ListAllEpisodes
	recAll := do(t, r, http.MethodGet, "/api/v1/episodes", nil, hdr)
	if recAll.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recAll.Code, recAll.Body)
	}
	var allResp []models.Episode
	if err := json.NewDecoder(recAll.Body).Decode(&allResp); err != nil {
		t.Fatal(err)
	}
	if len(allResp) != 2 {
		t.Errorf("expected 2 all episodes, got %d", len(allResp))
	}

	// Test GetEpisodeByID
	recGetID := do(t, r, http.MethodGet, fmt.Sprintf("/api/v1/episodes/%s", ep2.ID), nil, hdr)
	if recGetID.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recGetID.Code, recGetID.Body)
	}
	var getIDResp models.Episode
	if err := json.NewDecoder(recGetID.Body).Decode(&getIDResp); err != nil {
		t.Fatal(err)
	}
	if getIDResp.Title != "Ep 2" {
		t.Errorf("expected Ep 2, got %q", getIDResp.Title)
	}

	// Test DeleteEpisodeByID
	recDelID := do(t, r, http.MethodDelete, fmt.Sprintf("/api/v1/episodes/%s", ep2.ID), nil, hdr)
	if recDelID.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", recDelID.Code, recDelID.Body)
	}

	// Test DeleteEpisode
	delUrl := fmt.Sprintf("/api/v1/tv-shows/%s/seasons/1/episodes/%s", show.ID, ep1.ID)
	rec4 := do(t, r, http.MethodDelete, delUrl, nil, hdr)
	if rec4.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec4.Code, rec4.Body)
	}
}

