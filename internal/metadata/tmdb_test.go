package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tmdbMovieResponse(id int, title, releaseDate, overview, posterPath string) []byte {
	resp := map[string]any{
		"results": []map[string]any{
			{
				"id":           float64(id),
				"title":        title,
				"release_date": releaseDate,
				"overview":     overview,
				"poster_path":  posterPath,
			},
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func tmdbTVResponse(id int, name, firstAirDate, overview, posterPath string) []byte {
	resp := map[string]any{
		"results": []map[string]any{
			{
				"id":             float64(id),
				"name":           name,
				"first_air_date": firstAirDate,
				"overview":       overview,
				"poster_path":    posterPath,
			},
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func tmdbEmptyResponse() []byte {
	b, _ := json.Marshal(map[string]any{"results": []any{}})
	return b
}

func TestSearchMovie_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(tmdbMovieResponse(550, "Fight Club", "1999-10-15", "An insomniac office worker...", "/poster.jpg"))
	}))
	defer srv.Close()

	c := NewClient("test-key")
	c.baseURL = srv.URL

	result, err := c.SearchMovie(context.Background(), "Fight Club", 1999)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.ID != 550 {
		t.Errorf("ID: got %d, want 550", result.ID)
	}
	if result.Title != "Fight Club" {
		t.Errorf("Title: got %q, want %q", result.Title, "Fight Club")
	}
	if result.Year != 1999 {
		t.Errorf("Year: got %d, want 1999", result.Year)
	}
	if result.PosterPath != "/poster.jpg" {
		t.Errorf("PosterPath: got %q, want %q", result.PosterPath, "/poster.jpg")
	}
}

func TestSearchMovie_NoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(tmdbEmptyResponse())
	}))
	defer srv.Close()

	c := NewClient("test-key")
	c.baseURL = srv.URL

	result, err := c.SearchMovie(context.Background(), "NonExistent Film", 0)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestSearchMovie_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient("bad-key")
	c.baseURL = srv.URL

	_, err := c.SearchMovie(context.Background(), "Anything", 0)
	if err == nil {
		t.Error("expected error for HTTP 401, got nil")
	}
}

func TestSearchTV_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(tmdbTVResponse(1396, "Breaking Bad", "2008-01-20", "A teacher turns cook.", "/bbposter.jpg"))
	}))
	defer srv.Close()

	c := NewClient("test-key")
	c.baseURL = srv.URL

	result, err := c.SearchTV(context.Background(), "Breaking Bad", 0)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.ID != 1396 {
		t.Errorf("ID: got %d, want 1396", result.ID)
	}
	if result.Year != 2008 {
		t.Errorf("Year: got %d, want 2008", result.Year)
	}
}

func TestSearchTV_WithYear(t *testing.T) {
	var requestedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(tmdbTVResponse(1396, "Breaking Bad", "2008-01-20", "A teacher turns cook.", "/bbposter.jpg"))
	}))
	defer srv.Close()

	c := NewClient("test-key")
	c.baseURL = srv.URL

	_, err := c.SearchTV(context.Background(), "Breaking Bad", 2008)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(requestedURL, "first_air_date_year=2008") {
		t.Errorf("expected URL to contain first_air_date_year=2008, got %q", requestedURL)
	}
}

func TestDownloadPoster_Success(t *testing.T) {
	imageData := []byte("FAKEIMAGE")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(imageData)
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := NewClient("test-key")
	c.imageURL = srv.URL

	localPath, err := c.DownloadPoster(context.Background(), "/poster.jpg", dir)
	if err != nil {
		t.Fatal(err)
	}
	if localPath == "" {
		t.Fatal("expected non-empty local path")
	}
	got, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(imageData) {
		t.Errorf("file contents mismatch: got %q, want %q", got, imageData)
	}
	if filepath.Dir(localPath) != dir {
		t.Errorf("poster not in thumbs dir: %s", localPath)
	}
}

func TestDownloadPoster_EmptyPath(t *testing.T) {
	c := NewClient("test-key")
	path, err := c.DownloadPoster(context.Background(), "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		t.Errorf("expected empty path, got %q", path)
	}
}

func TestDownloadActorImage_Success(t *testing.T) {
	imageData := []byte("ACTORIMAGEBYTES")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(imageData)
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := NewClient("test-key")
	c.imageURL = srv.URL

	serverPath, err := c.DownloadActorImage(context.Background(), "/actor123.jpg", dir)
	if err != nil {
		t.Fatal(err)
	}
	if serverPath != "/api/v1/actors/image/actor123.jpg" {
		t.Errorf("expected /api/v1/actors/image/actor123.jpg, got %q", serverPath)
	}

	diskFile := filepath.Join(dir, "actor_actor123.jpg")
	got, err := os.ReadFile(diskFile)
	if err != nil {
		t.Fatalf("expected actor_actor123.jpg to be written: %v", err)
	}
	if string(got) != string(imageData) {
		t.Errorf("content mismatch: got %q, want %q", got, imageData)
	}

	// Secondary call should skip download because file exists
	serverPath2, err := c.DownloadActorImage(context.Background(), "/actor123.jpg", dir)
	if err != nil {
		t.Fatal(err)
	}
	if serverPath2 != "/api/v1/actors/image/actor123.jpg" {
		t.Errorf("expected /api/v1/actors/image/actor123.jpg on second call, got %q", serverPath2)
	}
}

func TestSearchTV_FiltersByYear(t *testing.T) {
	// Return two shows: first is "Breaking Bad" (2010), second is "Breaking Bad" (2008).
	// If we filter with 2008, it should choose the second show instead of the first show.
	tvMultipleResp, _ := json.Marshal(map[string]any{
		"results": []map[string]any{
			{
				"id":             float64(9999),
				"name":           "Breaking Bad",
				"first_air_date": "2010-01-20",
				"overview":       "2010 version...",
				"poster_path":    "",
			},
			{
				"id":             float64(1396),
				"name":           "Breaking Bad",
				"first_air_date": "2008-01-20",
				"overview":       "2008 version...",
				"poster_path":    "",
			},
		},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(tvMultipleResp)
	}))
	defer srv.Close()

	c := NewClient("test-key")
	c.baseURL = srv.URL

	result, err := c.SearchTV(context.Background(), "Breaking Bad", 2008)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.ID != 1396 {
		t.Errorf("expected to choose 2008 show (ID 1396), got ID %d", result.ID)
	}
}

func TestSearchMovieCandidates(t *testing.T) {
	movieMultipleResp, _ := json.Marshal(map[string]any{
		"results": []map[string]any{
			{
				"id":           float64(100),
				"title":        "Jurassic World",
				"release_date": "2015-06-12",
				"popularity":   85.5,
			},
			{
				"id":           float64(200),
				"title":        "Jurassic World Rebirth",
				"release_date": "2025-07-02",
				"popularity":   50.2,
			},
		},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(movieMultipleResp)
	}))
	defer srv.Close()

	c := NewClient("test-key")
	c.baseURL = srv.URL

	results, err := c.SearchMovieCandidates(context.Background(), "Jurassic World", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(results))
	}
	if results[0].ID != 100 || results[0].Popularity != 85.5 {
		t.Errorf("unexpected first candidate: %+v", results[0])
	}
	if results[1].ID != 200 || results[1].Popularity != 50.2 {
		t.Errorf("unexpected second candidate: %+v", results[1])
	}
}

func TestGetMovieDetails_Runtime(t *testing.T) {
	movieDetailsResp, _ := json.Marshal(map[string]any{
		"id":           float64(100),
		"title":        "Jurassic World",
		"release_date": "2015-06-12",
		"runtime":      124,
		"overview":     "A new theme park...",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(movieDetailsResp)
	}))
	defer srv.Close()

	c := NewClient("test-key")
	c.baseURL = srv.URL

	details, err := c.GetMovieDetails(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if details == nil {
		t.Fatal("expected details, got nil")
	}
	if details.Runtime != 124 {
		t.Errorf("expected runtime 124, got %d", details.Runtime)
	}
}

