package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ringmaster217/prism/internal/models"
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

// TMDBEpisodeResult holds episode-level fields from the TMDB episode endpoint.
type TMDBEpisodeResult struct {
	ID            int
	Name          string
	Overview      string
	StillPath     string
	SeasonNumber  int
	EpisodeNumber int
	AirYear       int
}

// TMDBSeasonResult holds season-level fields from the TMDB season endpoint.
type TMDBSeasonResult struct {
	ID           int
	Name         string
	Overview     string
	PosterPath   string
	SeasonNumber int
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
	result, err := c.search(ctx, "/search/movie", params, "title", "release_date")
	if err != nil {
		return nil, fmt.Errorf("search movie %q (year=%d): %w", title, year, err)
	}
	return result, nil
}

// SearchTV queries the TMDB TV search endpoint.
// Returns (nil, nil) when no results are found.
func (c *Client) SearchTV(ctx context.Context, title string) (*TMDBResult, error) {
	params := url.Values{"query": {title}, "api_key": {c.apiKey}}
	result, err := c.search(ctx, "/search/tv", params, "name", "first_air_date")
	if err != nil {
		return nil, fmt.Errorf("search tv %q: %w", title, err)
	}
	return result, nil
}

// GetTVSeason fetches season-level metadata from TMDB using the show's TMDB
// ID and the season number.
func (c *Client) GetTVSeason(ctx context.Context, showTMDBID, seasonNumber int) (*TMDBSeasonResult, error) {
	path := fmt.Sprintf("/tv/%d/season/%d", showTMDBID, seasonNumber)
	reqURL := c.baseURL + path + "?api_key=" + url.QueryEscape(c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	slog.Info("TMDB GET", "url", reqURL)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TMDB request %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB: HTTP %d for %s", resp.StatusCode, path)
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding TMDB season response for %s: %w", path, err)
	}

	r := &TMDBSeasonResult{}
	if id, ok := raw["id"].(float64); ok {
		r.ID = int(id)
	}
	if n, ok := raw["name"].(string); ok {
		r.Name = n
	}
	if o, ok := raw["overview"].(string); ok {
		r.Overview = o
	}
	if p, ok := raw["poster_path"].(string); ok {
		r.PosterPath = p
	}
	if sn, ok := raw["season_number"].(float64); ok {
		r.SeasonNumber = int(sn)
	}
	return r, nil
}

// GetTVEpisode fetches episode-level metadata from TMDB.
func (c *Client) GetTVEpisode(ctx context.Context, showTMDBID, seasonNumber, episodeNumber int) (*TMDBEpisodeResult, error) {
	path := fmt.Sprintf("/tv/%d/season/%d/episode/%d", showTMDBID, seasonNumber, episodeNumber)
	reqURL := c.baseURL + path + "?api_key=" + url.QueryEscape(c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	slog.Info("TMDB GET", "url", reqURL)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TMDB request %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB: HTTP %d for %s", resp.StatusCode, path)
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding TMDB episode response for %s: %w", path, err)
	}

	r := &TMDBEpisodeResult{}
	if id, ok := raw["id"].(float64); ok {
		r.ID = int(id)
	}
	if n, ok := raw["name"].(string); ok {
		r.Name = n
	}
	if o, ok := raw["overview"].(string); ok {
		r.Overview = o
	}
	if s, ok := raw["still_path"].(string); ok {
		r.StillPath = s
	}
	if sn, ok := raw["season_number"].(float64); ok {
		r.SeasonNumber = int(sn)
	}
	if en, ok := raw["episode_number"].(float64); ok {
		r.EpisodeNumber = int(en)
	}
	if d, ok := raw["air_date"].(string); ok && len(d) >= 4 {
		y, _ := strconv.Atoi(d[:4])
		r.AirYear = y
	}
	return r, nil
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
	slog.Info("TMDB GET", "url", imageURL)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("TMDB image server: HTTP %d", resp.StatusCode)
	}

	localName := strings.TrimPrefix(posterPath, "/")
	localPath := filepath.Join(destDir, localName)

	f, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("creating poster file: %w", err)
	}
	defer func() { _ = f.Close() }()

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
	slog.Info("TMDB GET", "url", reqURL)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TMDB request %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB: HTTP %d for %s", resp.StatusCode, path)
	}

	var sr tmdbSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decoding TMDB response for %s: %w", path, err)
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

// TMDBMovieDetails holds movie details fetched via get movie endpoint.
type TMDBMovieDetails struct {
	ID           int
	Title        string
	Year         int
	Overview     string
	PosterPath   string
	BackdropPath string
	Director     string
	Cast         []models.CastMember
	ExtraPosters []string
}

// TMDBTVDetails holds TV show details fetched via get tv endpoint.
type TMDBTVDetails struct {
	ID           int
	Name         string
	Year         int
	Overview     string
	PosterPath   string
	BackdropPath string
	Director     string
	Cast         []models.CastMember
	ExtraPosters []string
}

// GetMovieDetails fetches full movie details including credits and images from TMDB.
func (c *Client) GetMovieDetails(ctx context.Context, tmdbID int) (*TMDBMovieDetails, error) {
	path := fmt.Sprintf("/movie/%d", tmdbID)
	reqURL := fmt.Sprintf("%s%s?api_key=%s&append_to_response=credits,images", c.baseURL, path, url.QueryEscape(c.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	slog.Info("TMDB GET", "url", reqURL)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TMDB movie details request %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB: HTTP %d for %s", resp.StatusCode, path)
	}

	var raw struct {
		ID           int    `json:"id"`
		Title        string `json:"title"`
		ReleaseDate  string `json:"release_date"`
		Overview     string `json:"overview"`
		PosterPath   string `json:"poster_path"`
		BackdropPath string `json:"backdrop_path"`
		Credits      *struct {
			Cast []struct {
				Name        string  `json:"name"`
				Character   string  `json:"character"`
				ProfilePath *string `json:"profile_path"`
			} `json:"cast"`
			Crew []struct {
				Name string `json:"name"`
				Job  string `json:"job"`
			} `json:"crew"`
		} `json:"credits"`
		Images *struct {
			Posters []struct {
				FilePath string `json:"file_path"`
			} `json:"posters"`
		} `json:"images"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding TMDB response for %s: %w", path, err)
	}

	details := &TMDBMovieDetails{
		ID:           raw.ID,
		Title:        raw.Title,
		Overview:     raw.Overview,
		PosterPath:   raw.PosterPath,
		BackdropPath: raw.BackdropPath,
	}

	if len(raw.ReleaseDate) >= 4 {
		y, _ := strconv.Atoi(raw.ReleaseDate[:4])
		details.Year = y
	}

	// Extract director from crew
	if raw.Credits != nil {
		var directors []string
		for _, member := range raw.Credits.Crew {
			if member.Job == "Director" {
				directors = append(directors, member.Name)
			}
		}
		details.Director = strings.Join(directors, ", ")

		// Extract top 10 cast members
		castCount := len(raw.Credits.Cast)
		if castCount > 10 {
			castCount = 10
		}
		for i := 0; i < castCount; i++ {
			cMember := raw.Credits.Cast[i]
			profileURL := ""
			if cMember.ProfilePath != nil {
				profileURL = "https://image.tmdb.org/t/p/w185" + *cMember.ProfilePath
			}
			details.Cast = append(details.Cast, models.CastMember{
				Name:        cMember.Name,
				Character:   cMember.Character,
				ProfilePath: profileURL,
			})
		}
	}

	// Extract extra posters (skip the main poster if it matches raw.PosterPath)
	if raw.Images != nil {
		count := 0
		for _, p := range raw.Images.Posters {
			if p.FilePath != "" && p.FilePath != raw.PosterPath {
				details.ExtraPosters = append(details.ExtraPosters, p.FilePath)
				count++
				if count >= 5 {
					break
				}
			}
		}
	}

	return details, nil
}

// GetTVDetails fetches full TV show details including credits and images from TMDB.
func (c *Client) GetTVDetails(ctx context.Context, tmdbID int) (*TMDBTVDetails, error) {
	path := fmt.Sprintf("/tv/%d", tmdbID)
	reqURL := fmt.Sprintf("%s%s?api_key=%s&append_to_response=credits,images", c.baseURL, path, url.QueryEscape(c.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	slog.Info("TMDB GET", "url", reqURL)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TMDB tv details request %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB: HTTP %d for %s", resp.StatusCode, path)
	}

	var raw struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		FirstAirDate string `json:"first_air_date"`
		Overview     string `json:"overview"`
		PosterPath   string `json:"poster_path"`
		BackdropPath string `json:"backdrop_path"`
		CreatedBy    []struct {
			Name string `json:"name"`
		} `json:"created_by"`
		Credits *struct {
			Cast []struct {
				Name        string  `json:"name"`
				Character   string  `json:"character"`
				ProfilePath *string `json:"profile_path"`
			} `json:"cast"`
			Crew []struct {
				Name string `json:"name"`
				Job  string `json:"job"`
			} `json:"crew"`
		} `json:"credits"`
		Images *struct {
			Posters []struct {
				FilePath string `json:"file_path"`
			} `json:"posters"`
		} `json:"images"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding TMDB response for %s: %w", path, err)
	}

	details := &TMDBTVDetails{
		ID:           raw.ID,
		Name:         raw.Name,
		Overview:     raw.Overview,
		PosterPath:   raw.PosterPath,
		BackdropPath: raw.BackdropPath,
	}

	if len(raw.FirstAirDate) >= 4 {
		y, _ := strconv.Atoi(raw.FirstAirDate[:4])
		details.Year = y
	}

	// Extract director / creator
	var creators []string
	for _, creator := range raw.CreatedBy {
		creators = append(creators, creator.Name)
	}
	if len(creators) > 0 {
		details.Director = strings.Join(creators, ", ")
	} else if raw.Credits != nil {
		var producers []string
		for _, member := range raw.Credits.Crew {
			if member.Job == "Executive Producer" || member.Job == "Director" {
				producers = append(producers, member.Name)
			}
		}
		if len(producers) > 0 {
			details.Director = strings.Join(producers, ", ")
		}
	}

	// Extract top 10 cast members
	if raw.Credits != nil {
		castCount := len(raw.Credits.Cast)
		if castCount > 10 {
			castCount = 10
		}
		for i := 0; i < castCount; i++ {
			cMember := raw.Credits.Cast[i]
			profileURL := ""
			if cMember.ProfilePath != nil {
				profileURL = "https://image.tmdb.org/t/p/w185" + *cMember.ProfilePath
			}
			details.Cast = append(details.Cast, models.CastMember{
				Name:        cMember.Name,
				Character:   cMember.Character,
				ProfilePath: profileURL,
			})
		}
	}

	// Extract extra posters
	if raw.Images != nil {
		count := 0
		for _, p := range raw.Images.Posters {
			if p.FilePath != "" && p.FilePath != raw.PosterPath {
				details.ExtraPosters = append(details.ExtraPosters, p.FilePath)
				count++
				if count >= 5 {
					break
				}
			}
		}
	}

	return details, nil
}
