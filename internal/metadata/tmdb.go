package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	tmdbBaseURL  = "https://api.themoviedb.org/3"
	tmdbImageURL = "https://image.tmdb.org/t/p/w500"
)

// TMDBResult holds the fields we care about from a TMDB search result.
type TMDBResult struct {
	ID         int
	Title      string
	Year       int
	Overview   string
	PosterPath string
}

// Client is a thin TMDB API v3 HTTP client.
type Client struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
	imageURL   string
}

// NewClient creates a TMDB client keyed by apiKey.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    tmdbBaseURL,
		imageURL:   tmdbImageURL,
	}
}

// SearchMovie queries the TMDB movie search endpoint. Use year=0 for no year
// filter. Returns (nil, nil) when no results are found.
func (c *Client) SearchMovie(ctx context.Context, title string, year int) (*TMDBResult, error) {
	params := url.Values{"query": {title}, "api_key": {c.apiKey}}
	if year != 0 {
		params.Set("year", strconv.Itoa(year))
	}
	return c.search(ctx, "/search/movie", params, "title", "release_date")
}

// SearchTV queries the TMDB TV search endpoint.
// Returns (nil, nil) when no results are found.
func (c *Client) SearchTV(ctx context.Context, title string) (*TMDBResult, error) {
	params := url.Values{"query": {title}, "api_key": {c.apiKey}}
	return c.search(ctx, "/search/tv", params, "name", "first_air_date")
}

// DownloadPoster downloads a TMDB poster image (e.g. "/abc123.jpg") into
// destDir and returns the local absolute path. Returns ("", nil) when
// posterPath is empty.
func (c *Client) DownloadPoster(ctx context.Context, posterPath, destDir string) (string, error) {
	if posterPath == "" {
		return "", nil
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("creating thumbs dir: %w", err)
	}

	imageURL := c.imageURL + posterPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("TMDB image server: HTTP %d", resp.StatusCode)
	}

	localName := strings.TrimPrefix(posterPath, "/")
	localPath := filepath.Join(destDir, localName)

	f, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("creating poster file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("writing poster: %w", err)
	}
	return localPath, nil
}

// --- internal helpers ---

type tmdbSearchResponse struct {
	Results []json.RawMessage `json:"results"`
}

// search performs a parameterised TMDB search and unmarshals the first result.
func (c *Client) search(ctx context.Context, path string, params url.Values, titleKey, dateKey string) (*TMDBResult, error) {
	reqURL := c.baseURL + path + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TMDB request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB: HTTP %d for %s", resp.StatusCode, path)
	}

	var sr tmdbSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decoding TMDB response: %w", err)
	}
	if len(sr.Results) == 0 {
		return nil, nil
	}

	var raw map[string]any
	if err := json.Unmarshal(sr.Results[0], &raw); err != nil {
		return nil, err
	}

	r := &TMDBResult{}
	if id, ok := raw["id"].(float64); ok {
		r.ID = int(id)
	}
	if t, ok := raw[titleKey].(string); ok {
		r.Title = t
	}
	if o, ok := raw["overview"].(string); ok {
		r.Overview = o
	}
	if p, ok := raw["poster_path"].(string); ok {
		r.PosterPath = p
	}
	if d, ok := raw[dateKey].(string); ok && len(d) >= 4 {
		y, _ := strconv.Atoi(d[:4])
		r.Year = y
	}
	return r, nil
}
