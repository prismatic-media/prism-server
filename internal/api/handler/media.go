package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/artifact"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

// MediaHandler handles media item queries and deletions.
type MediaHandler struct {
	db *sql.DB
}

func NewMediaHandler(db *sql.DB) *MediaHandler {
	return &MediaHandler{db: db}
}

// ListMedia returns all media items. If a library_id query parameter is
// supplied, only items for that library are returned.
// @Summary List Media Items
// @Description Lists media items (movies, episodes) with filtering/sorting capabilities.
// @Tags Media Items
// @Security BearerAuth
// @Produce json
// @Param library_id query string false "Library ID" format(uuid)
// @Param media_type query string false "Media Type" enum(movie,episode)
// @Param sort query string false "Sort order" enum(recent)
// @Param limit query integer false "Recent items count limit" default(20)
// @Success 200 {array} models.MediaItem
// @Failure 400 {object} map[string]string "Invalid library ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Router /media [get]
func (h *MediaHandler) ListMedia(w http.ResponseWriter, r *http.Request) {
	sortStr := r.URL.Query().Get("sort")
	if sortStr == "recent" {
		limit := 20
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		items, err := sqlite.ListRecentMediaItems(r.Context(), h.db, limit)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not list recent media items", err)
			return
		}
		respondJSON(w, http.StatusOK, emptySlice(items))
		return
	}

	libIDStr := r.URL.Query().Get("library_id")
	allStr := r.URL.Query().Get("all")

	if libIDStr != "" {
		libID, err := uuid.Parse(libIDStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid library_id")
			return
		}
		var items []*models.MediaItem
		if allStr == "true" {
			items, err = sqlite.ListMediaItemsAll(r.Context(), h.db, libID)
		} else {
			items, err = sqlite.ListMediaItems(r.Context(), h.db, libID)
		}
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not list media items", err)
			return
		}
		respondJSON(w, http.StatusOK, emptySlice(items))
		return
	}

	var items []*models.MediaItem
	var err error
	if allStr == "true" {
		items, err = sqlite.ListAllMediaItemsAll(r.Context(), h.db)
	} else {
		items, err = sqlite.ListAllMediaItems(r.Context(), h.db)
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list media items", err)
		return
	}
	respondJSON(w, http.StatusOK, emptySlice(items))
}

// Search queries media items and TV shows matching a search string.
// @Summary Global Search
// @Description Searches library for shows and movies by title.
// @Tags Media Items
// @Security BearerAuth
// @Produce json
// @Param q query string true "Search query string"
// @Success 200 {array} models.SearchResult
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Router /search [get]
func (h *MediaHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		respondJSON(w, http.StatusOK, []any{})
		return
	}

	results, err := sqlite.SearchMedia(r.Context(), h.db, q)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not execute search", err)
		return
	}

	respondJSON(w, http.StatusOK, emptySlice(results))
}

// GetMedia returns a single media item by ID.
// @Summary Get Media Item
// @Tags Media Items
// @Security BearerAuth
// @Produce json
// @Param id path string true "Media ID" format(uuid)
// @Success 200 {object} models.MediaItem
// @Failure 400 {object} map[string]string "Invalid media ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 404 {object} map[string]string "Media item not found"
// @Router /media/{id} [get]
func (h *MediaHandler) GetMedia(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id")
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	respondJSON(w, http.StatusOK, item)
}

// DeleteMedia removes a media item (admin only).
// @Summary Delete Media Item (Admin Only)
// @Tags Media Items
// @Security BearerAuth
// @Param id path string true "Media ID" format(uuid)
// @Success 204 "Media item deleted successfully"
// @Failure 400 {object} map[string]string "Invalid media ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Failure 404 {object} map[string]string "Media item not found"
// @Router /media/{id} [delete]
func (h *MediaHandler) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id")
		return
	}

	if err := sqlite.DeleteMediaItem(r.Context(), h.db, id); errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found")
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "could not delete media item", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ServePoster serves the cached poster image for a media item.
func (h *MediaHandler) ServePoster(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id")
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	if item.PosterPath == nil || *item.PosterPath == "" {
		respondError(w, http.StatusNotFound, "no poster available")
		return
	}
	http.ServeFile(w, r, *item.PosterPath)
}

// ServeBackdrop serves the cached backdrop image for a media item.
func (h *MediaHandler) ServeBackdrop(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id")
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	if item.BackdropPath == nil || *item.BackdropPath == "" {
		respondError(w, http.StatusNotFound, "no backdrop available")
		return
	}
	http.ServeFile(w, r, *item.BackdropPath)
}

// ServeExtraPoster serves a cached extra poster image for a media item by index.
func (h *MediaHandler) ServeExtraPoster(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id")
		return
	}

	indexStr := chi.URLParam(r, "index")
	index, err := strconv.Atoi(indexStr)
	if err != nil || index < 0 {
		respondError(w, http.StatusBadRequest, "invalid extra poster index")
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	if len(item.ExtraPosters) == 0 || index >= len(item.ExtraPosters) {
		respondError(w, http.StatusNotFound, "extra poster index out of bounds")
		return
	}
	http.ServeFile(w, r, item.ExtraPosters[index])
}

// GetTranscodeSizes returns the size of each resolution and the total size in the transcode bundle.
// @Summary Get Media Transcode Sizes
// @Description Returns the size of each resolution subdirectory and the total size in the media item's transcode bundle.
// @Tags Media Items
// @Security BearerAuth
// @Produce json
// @Param id path string true "Media ID" format(uuid)
// @Success 200 {object} models.TranscodeSizesInfo
// @Failure 400 {object} map[string]string "Invalid media ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 404 {object} map[string]string "Media item not found"
// @Router /media/{id}/transcode-sizes [get]
func (h *MediaHandler) GetTranscodeSizes(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id")
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	if item.MPDPath == nil || *item.MPDPath == "" {
		respondJSON(w, http.StatusOK, models.TranscodeSizesInfo{Renditions: []models.RenditionSize{}})
		return
	}

	if item.TranscodeSizes != nil && len(item.TranscodeSizes.Renditions) > 0 {
		sortRenditions(item.TranscodeSizes.Renditions)
		respondJSON(w, http.StatusOK, item.TranscodeSizes)
		return
	}

	outputDir := filepath.Dir(*item.MPDPath)
	// Try reading from sidecar
	if meta, err := artifact.ReadSidecar(outputDir); err == nil && meta != nil && meta.Sizes != nil && len(meta.Sizes.Renditions) > 0 {
		sortRenditions(meta.Sizes.Renditions)
		// Update DB with sidecar sizes
		_ = sqlite.SetMediaTranscodeSizes(r.Context(), h.db, item.ID, meta.Sizes)
		respondJSON(w, http.StatusOK, meta.Sizes)
		return
	}

	// Fallback to dynamic scanning
	sizes := artifact.GetTranscodeSizesInfo(*item.MPDPath)
	sortRenditions(sizes.Renditions)

	// Cache to database
	_ = sqlite.SetMediaTranscodeSizes(r.Context(), h.db, item.ID, &sizes)

	// Cache to sidecar if sidecar exists
	if meta, err := artifact.ReadSidecar(outputDir); err == nil && meta != nil {
		meta.Sizes = &sizes
		_ = artifact.WriteSidecar(outputDir, meta)
	}

	respondJSON(w, http.StatusOK, sizes)
}

func sortRenditions(renditions []models.RenditionSize) {
	sort.Slice(renditions, func(i, j int) bool {
		valI := parseResolution(renditions[i].Resolution)
		valJ := parseResolution(renditions[j].Resolution)
		return valI < valJ
	})
}

func parseResolution(res string) int {
	resLower := strings.ToLower(res)
	if strings.HasPrefix(resLower, "4k") {
		return 2160
	}
	if strings.HasPrefix(resLower, "8k") {
		return 4320
	}
	var digits strings.Builder
	for _, ch := range res {
		if ch >= '0' && ch <= '9' {
			digits.WriteRune(ch)
		} else {
			break
		}
	}
	if digits.Len() > 0 {
		val, _ := strconv.Atoi(digits.String())
		return val
	}
	return 0
}

// emptySlice converts a nil slice to an empty interface slice for JSON
// serialisation — ensures the response is [] rather than null.
func emptySlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

