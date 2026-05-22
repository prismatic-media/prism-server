package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	apimw "github.com/ringmaster217/galactic-media-server/internal/api/middleware"
	"github.com/ringmaster217/galactic-media-server/internal/models"
	"github.com/ringmaster217/galactic-media-server/internal/store/sqlite"
)

// HistoryHandler manages watch-history reads and writes.
type HistoryHandler struct {
	db *sql.DB
}

func NewHistoryHandler(db *sql.DB) *HistoryHandler {
	return &HistoryHandler{db: db}
}

type upsertHistoryRequest struct {
	Position  float64 `json:"position"`
	Completed bool    `json:"completed"`
}

// GetHistory handles GET /api/v1/history.
// Returns in-progress (not completed) watch history for the authenticated user.
func (h *HistoryHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	claims := apimw.ClaimsFromContext(r.Context())
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "invalid user id in token")
		return
	}

	items, err := sqlite.ListWatchHistory(r.Context(), h.db, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch watch history")
		return
	}
	if items == nil {
		items = []*models.WatchHistory{}
	}
	respondJSON(w, http.StatusOK, items)
}

// UpsertHistory handles PUT /api/v1/history/{mediaID}.
// Creates or updates the watch position for the authenticated user and the
// given media item. Clients should call this periodically (e.g. every 10 s).
func (h *HistoryHandler) UpsertHistory(w http.ResponseWriter, r *http.Request) {
	claims := apimw.ClaimsFromContext(r.Context())
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "invalid user id in token")
		return
	}

	mediaID, err := uuidParam(r, "mediaID")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id")
		return
	}

	var req upsertHistoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Verify the media item exists.
	_, err = sqlite.GetMediaItemByID(r.Context(), h.db, mediaID)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not verify media item")
		return
	}

	entry := &models.WatchHistory{
		UserID:      userID,
		MediaItemID: mediaID,
		Position:    req.Position,
		Completed:   req.Completed,
	}
	if err := sqlite.UpsertWatchHistory(r.Context(), h.db, entry); err != nil {
		respondError(w, http.StatusInternalServerError, "could not save watch history")
		return
	}

	respondJSON(w, http.StatusOK, entry)
}
