package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/scanner"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

// LibraryHandler handles CRUD operations on libraries and triggers scans.
type LibraryHandler struct {
	db      *sql.DB
	manager *scanner.Manager
}

func NewLibraryHandler(db *sql.DB, manager *scanner.Manager) *LibraryHandler {
	return &LibraryHandler{db: db, manager: manager}
}

type createLibraryRequest struct {
	Path      string `json:"path"`
	MediaType string `json:"media_type"`
}

// CreateLibrary registers a new media library directory.
// Requires admin.
// @Summary Create Library (Admin Only)
// @Description Add a new media library pointing to a directory path.
// @Tags Libraries
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body createLibraryRequest true "Library creation properties"
// @Success 201 {object} models.Library
// @Failure 400 {object} map[string]string "Invalid request body or missing fields"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Failure 409 {object} map[string]string "Library path already registered"
// @Router /libraries [post]
func (h *LibraryHandler) CreateLibrary(w http.ResponseWriter, r *http.Request) {
	var req createLibraryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}
	if req.Path == "" || req.MediaType == "" {
		respondError(w, http.StatusBadRequest, "path and media_type are required")
		return
	}

	mt := models.MediaType(req.MediaType)
	switch mt {
	case models.MediaTypeMovie, models.MediaTypeTVShow, models.MediaTypeMusic:
	default:
		respondError(w, http.StatusBadRequest, "media_type must be one of: movie, tvshow, music")
		return
	}

	lib := &models.Library{
		Path:      req.Path,
		MediaType: mt,
	}
	if err := sqlite.CreateLibrary(r.Context(), h.db, lib); err != nil {
		respondError(w, http.StatusConflict, "library path already registered", err)
		return
	}

	// Start watching the new library in the background.
	h.manager.Add(r.Context(), lib)

	respondJSON(w, http.StatusCreated, lib)
}

// ListLibraries returns all registered libraries.
// @Summary List Libraries
// @Description Retrieve a list of all libraries (movie, tvshow, etc.) indexed by the server.
// @Tags Libraries
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.Library
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Router /libraries [get]
func (h *LibraryHandler) ListLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := sqlite.ListLibraries(r.Context(), h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list libraries", err)
		return
	}
	if libs == nil {
		libs = []*models.Library{} // always return array, never null
	}
	respondJSON(w, http.StatusOK, libs)
}

// GetLibrary returns a single library by ID.
// @Summary Get Library Details
// @Tags Libraries
// @Security BearerAuth
// @Produce json
// @Param id path string true "Library ID" format(uuid)
// @Success 200 {object} models.Library
// @Failure 400 {object} map[string]string "Invalid library ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 404 {object} map[string]string "Library not found"
// @Router /libraries/{id} [get]
func (h *LibraryHandler) GetLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid library id", err)
		return
	}

	lib, err := sqlite.GetLibraryByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "library not found", err)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch library", err)
		return
	}

	respondJSON(w, http.StatusOK, lib)
}

// DeleteLibrary removes a library and all its media items (admin only).
// @Summary Delete Library (Admin Only)
// @Tags Libraries
// @Security BearerAuth
// @Param id path string true "Library ID" format(uuid)
// @Success 204 "Library deleted successfully"
// @Failure 400 {object} map[string]string "Invalid library ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Failure 404 {object} map[string]string "Library not found"
// @Router /libraries/{id} [delete]
func (h *LibraryHandler) DeleteLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid library id", err)
		return
	}

	if err := sqlite.DeleteLibrary(r.Context(), h.db, id); errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "library not found", err)
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "could not delete library", err)
		return
	}

	h.manager.Remove(id)
	w.WriteHeader(http.StatusNoContent)
}

// ScanLibrary triggers an immediate full scan of a library (admin only).
// @Summary Scan Library (Admin Only)
// @Description Triggers a directory scan to index new/updated files.
// @Tags Libraries
// @Security BearerAuth
// @Produce json
// @Param id path string true "Library ID" format(uuid)
// @Success 202 {object} map[string]string "Library scan triggered"
// @Failure 400 {object} map[string]string "Invalid library ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Failure 404 {object} map[string]string "Library not found"
// @Router /libraries/{id}/scan [post]
func (h *LibraryHandler) ScanLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid library id", err)
		return
	}

	// Verify library exists.
	if _, err := sqlite.GetLibraryByID(r.Context(), h.db, id); errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "library not found", err)
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch library", err)
		return
	}

	// Run scan in background so the HTTP response is immediate.
	go func() {
		// Context may be cancelled after response is sent — that is fine.
		_ = h.manager.Scan(r.Context(), id)
	}()

	respondJSON(w, http.StatusAccepted, map[string]string{"status": "scan started"})
}

// uuidParam extracts and parses a chi URL parameter as a UUID.
func uuidParam(r *http.Request, key string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, key))
}


