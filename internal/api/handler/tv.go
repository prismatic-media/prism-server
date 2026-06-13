package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

// TVHandler handles TV show/season/episode browsing.
type TVHandler struct {
	db *sql.DB
}

func NewTVHandler(db *sql.DB) *TVHandler {
	return &TVHandler{db: db}
}

// ListShows returns TV shows. If a library_id query parameter is
// supplied, only shows for that library are returned. Otherwise, returns
// all TV shows across all libraries.
// GET /api/v1/tv/shows
func (h *TVHandler) ListShows(w http.ResponseWriter, r *http.Request) {
	libIDStr := r.URL.Query().Get("library_id")
	sortStr := r.URL.Query().Get("sort")

	if sortStr == "recent" {
		limit := 20
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		shows, err := sqlite.ListRecentTVShows(r.Context(), h.db, limit)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not list recent tv shows", err)
			return
		}
		respondJSON(w, http.StatusOK, emptySlice(shows))
		return
	}

	if libIDStr == "" {
		shows, err := sqlite.ListAllTVShows(r.Context(), h.db)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not list all tv shows", err)
			return
		}
		respondJSON(w, http.StatusOK, emptySlice(shows))
		return
	}

	libID, err := uuid.Parse(libIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid library_id")
		return
	}

	shows, err := sqlite.ListTVShows(r.Context(), h.db, libID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list shows", err)
		return
	}
	respondJSON(w, http.StatusOK, emptySlice(shows))
}

// GetShow returns a single TV show by ID.
// GET /api/v1/tv/shows/{id}
func (h *TVHandler) GetShow(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid show id")
		return
	}

	show, err := sqlite.GetTVShowByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "show not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch show", err)
		return
	}
	respondJSON(w, http.StatusOK, show)
}

// ListSeasons returns all seasons for a TV show.
// GET /api/v1/tv/shows/{id}/seasons
func (h *TVHandler) ListSeasons(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid show id")
		return
	}

	seasons, err := sqlite.ListTVSeasons(r.Context(), h.db, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list seasons", err)
		return
	}
	respondJSON(w, http.StatusOK, emptySlice(seasons))
}

// ListEpisodes returns all episodes for a specific season.
// GET /api/v1/tv/shows/{id}/seasons/{number}/episodes
func (h *TVHandler) ListEpisodes(w http.ResponseWriter, r *http.Request) {
	showID, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid show id")
		return
	}

	seasonNumStr := chi.URLParam(r, "number")
	seasonNum, err := strconv.Atoi(seasonNumStr)
	if err != nil || seasonNum < 1 {
		respondError(w, http.StatusBadRequest, "invalid season number")
		return
	}

	season, err := sqlite.GetTVSeasonByShowAndNumber(r.Context(), h.db, showID, seasonNum)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "season not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch season", err)
		return
	}

	episodes, err := sqlite.ListSeasonEpisodes(r.Context(), h.db, season.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list episodes", err)
		return
	}
	respondJSON(w, http.StatusOK, emptySlice(episodes))
}

// ServeShowPoster serves the cached poster image for a TV show.
// GET /api/v1/tv/shows/{id}/poster
func (h *TVHandler) ServeShowPoster(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid show id")
		return
	}

	show, err := sqlite.GetTVShowByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "show not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch show", err)
		return
	}
	if show.PosterPath == nil || *show.PosterPath == "" {
		respondError(w, http.StatusNotFound, "no poster available")
		return
	}

	posterPath := *show.PosterPath
	if !filepath.IsAbs(posterPath) {
		thumbsDir, _ := sqlite.GetSetting(r.Context(), h.db, "thumbs_dir")
		posterPath = filepath.Join(thumbsDir, posterPath)
	}
	if _, err := os.Stat(posterPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "poster file not found")
		return
	}
	http.ServeFile(w, r, posterPath)
}

// ServeSeasonPoster serves the poster for a TV season.
// GET /api/v1/tv/shows/{id}/seasons/{number}/poster
func (h *TVHandler) ServeSeasonPoster(w http.ResponseWriter, r *http.Request) {
	showID, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid show id")
		return
	}

	seasonNumStr := chi.URLParam(r, "number")
	seasonNum, err := strconv.Atoi(seasonNumStr)
	if err != nil || seasonNum < 1 {
		respondError(w, http.StatusBadRequest, "invalid season number")
		return
	}

	season, err := sqlite.GetTVSeasonByShowAndNumber(r.Context(), h.db, showID, seasonNum)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "season not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch season", err)
		return
	}
	if season.PosterPath == nil || *season.PosterPath == "" {
		respondError(w, http.StatusNotFound, "no poster available")
		return
	}

	posterPath := *season.PosterPath
	if !filepath.IsAbs(posterPath) {
		thumbsDir, _ := sqlite.GetSetting(r.Context(), h.db, "thumbs_dir")
		posterPath = filepath.Join(thumbsDir, posterPath)
	}
	if _, err := os.Stat(posterPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "poster file not found")
		return
	}
	http.ServeFile(w, r, posterPath)
}

// ServeShowBackdrop serves the cached backdrop image for a TV show.
// GET /api/v1/tv/shows/{id}/backdrop
func (h *TVHandler) ServeShowBackdrop(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid show id")
		return
	}

	show, err := sqlite.GetTVShowByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "show not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch show", err)
		return
	}
	if show.BackdropPath == nil || *show.BackdropPath == "" {
		respondError(w, http.StatusNotFound, "no backdrop available")
		return
	}

	backdropPath := *show.BackdropPath
	if !filepath.IsAbs(backdropPath) {
		thumbsDir, _ := sqlite.GetSetting(r.Context(), h.db, "thumbs_dir")
		backdropPath = filepath.Join(thumbsDir, backdropPath)
	}
	if _, err := os.Stat(backdropPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "backdrop file not found")
		return
	}
	http.ServeFile(w, r, backdropPath)
}

// ServeShowExtraPoster serves a cached extra poster image for a TV show by index.
// GET /api/v1/tv/shows/{id}/extra-posters/{index}
func (h *TVHandler) ServeShowExtraPoster(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid show id")
		return
	}

	indexStr := chi.URLParam(r, "index")
	index, err := strconv.Atoi(indexStr)
	if err != nil || index < 0 {
		respondError(w, http.StatusBadRequest, "invalid extra poster index")
		return
	}

	show, err := sqlite.GetTVShowByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "show not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch show", err)
		return
	}

	if len(show.ExtraPosters) == 0 || index >= len(show.ExtraPosters) {
		respondError(w, http.StatusNotFound, "extra poster index out of bounds")
		return
	}

	posterPath := show.ExtraPosters[index]
	if !filepath.IsAbs(posterPath) {
		thumbsDir, _ := sqlite.GetSetting(r.Context(), h.db, "thumbs_dir")
		posterPath = filepath.Join(thumbsDir, posterPath)
	}
	if _, err := os.Stat(posterPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "poster file not found")
		return
	}
	http.ServeFile(w, r, posterPath)
}
