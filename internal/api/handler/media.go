package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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
			respondError(w, http.StatusInternalServerError, "could not list media items")
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
		respondError(w, http.StatusInternalServerError, "could not list media items")
		return
	}
	respondJSON(w, http.StatusOK, emptySlice(items))
}

// Search queries media items and TV shows matching a search string.
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
		respondError(w, http.StatusInternalServerError, "could not fetch media item")
		return
	}

	respondJSON(w, http.StatusOK, item)
}

// DeleteMedia removes a media item (admin only).
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
		respondError(w, http.StatusInternalServerError, "could not delete media item")
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
		respondError(w, http.StatusInternalServerError, "could not fetch media item")
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
		respondError(w, http.StatusInternalServerError, "could not fetch media item")
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
		respondError(w, http.StatusInternalServerError, "could not fetch media item")
		return
	}

	if len(item.ExtraPosters) == 0 || index >= len(item.ExtraPosters) {
		respondError(w, http.StatusNotFound, "extra poster index out of bounds")
		return
	}
	http.ServeFile(w, r, item.ExtraPosters[index])
}


// emptySlice converts a nil slice to an empty interface slice for JSON
// serialisation — ensures the response is [] rather than null.
func emptySlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

