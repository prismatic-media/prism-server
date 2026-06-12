package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	apimw "github.com/ringmaster217/prism/internal/api/middleware"
	"github.com/ringmaster217/prism/internal/models"
	"github.com/ringmaster217/prism/internal/scanner"
	"github.com/ringmaster217/prism/internal/store/sqlite"
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
func (h *LibraryHandler) CreateLibrary(w http.ResponseWriter, r *http.Request) {
	var req createLibraryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
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
		respondError(w, http.StatusConflict, "library path already registered")
		return
	}

	// Start watching the new library in the background.
	h.manager.Add(r.Context(), lib)

	respondJSON(w, http.StatusCreated, lib)
}

// ListLibraries returns all registered libraries.
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
func (h *LibraryHandler) GetLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid library id")
		return
	}

	lib, err := sqlite.GetLibraryByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "library not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch library", err)
		return
	}

	respondJSON(w, http.StatusOK, lib)
}

// DeleteLibrary removes a library and all its media items (admin only).
func (h *LibraryHandler) DeleteLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid library id")
		return
	}

	if err := sqlite.DeleteLibrary(r.Context(), h.db, id); errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "library not found")
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "could not delete library", err)
		return
	}

	h.manager.Remove(id)
	w.WriteHeader(http.StatusNoContent)
}

// ScanLibrary triggers an immediate full scan of a library (admin only).
func (h *LibraryHandler) ScanLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid library id")
		return
	}

	// Verify library exists.
	if _, err := sqlite.GetLibraryByID(r.Context(), h.db, id); errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "library not found")
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch library", err)
		return
	}

	// Run scan in background so the HTTP response is immediate.
	go func() {
		if err := h.manager.Scan(r.Context(), id); err != nil {
			// Context may be cancelled after response is sent — that is fine.
		}
	}()

	respondJSON(w, http.StatusAccepted, map[string]string{"status": "scan started"})
}

// uuidParam extracts and parses a chi URL parameter as a UUID.
func uuidParam(r *http.Request, key string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, key))
}

// requireAdmin checks that the request has admin claims. Returns false and
// writes a 403 if not. Convenience wrapper for handlers that need it without
// being in the RequireAdmin middleware chain.
func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	claims := apimw.ClaimsFromContext(r.Context())
	if claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "admin access required")
		return false
	}
	return true
}
