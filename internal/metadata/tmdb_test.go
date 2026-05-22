package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		w.Write(tmdbMovieResponse(550, "Fight Club", "1999-10-15", "An insomniac office worker...", "/poster.jpg"))
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
		w.Write(tmdbEmptyResponse())
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
		w.Write(tmdbTVResponse(1396, "Breaking Bad", "2008-01-20", "A teacher turns cook.", "/bbposter.jpg"))
	}))
	defer srv.Close()

	c := NewClient("test-key")
	c.baseURL = srv.URL

	result, err := c.SearchTV(context.Background(), "Breaking Bad")
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

func TestDownloadPoster_Success(t *testing.T) {
	imageData := []byte("FAKEIMAGE")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(imageData)
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
