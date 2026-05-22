package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/ringmaster217/galactic-media-server/internal/store/sqlite"
)

// MediaHandler handles media item queries and deletions.
type MediaHandler struct {
	db        *sql.DB
	thumbsDir string
}

func NewMediaHandler(db *sql.DB, thumbsDir string) *MediaHandler {
	return &MediaHandler{db: db, thumbsDir: thumbsDir}
}

// ListMedia returns all media items. If a library_id query parameter is
// supplied, only items for that library are returned.
func (h *MediaHandler) ListMedia(w http.ResponseWriter, r *http.Request) {
	libIDStr := r.URL.Query().Get("library_id")

	if libIDStr != "" {
		libID, err := uuid.Parse(libIDStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid library_id")
			return
		}
		items, err := sqlite.ListMediaItems(r.Context(), h.db, libID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not list media items")
			return
		}
		if items == nil {
			items = nil // keep nil — frontend can handle empty array
		}
		respondJSON(w, http.StatusOK, emptySlice(items))
		return
	}

	items, err := sqlite.ListAllMediaItems(r.Context(), h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list media items")
		return
	}
	respondJSON(w, http.StatusOK, emptySlice(items))
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

// emptySlice converts a nil slice to an empty interface slice for JSON
// serialisation — ensures the response is [] rather than null.
func emptySlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
