package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	apimw "github.com/prismatic-media/prism-server/internal/api/middleware"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

// nowPlayingResponse bundles a watch-history entry with its media item so the
// client needs only one request to render the Now Playing bar.
type nowPlayingResponse struct {
	History *models.WatchHistory `json:"history"`
	Media   *models.MediaItem    `json:"media"`
}

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
// @Summary Get User Playback History
// @Description Retrieve a list of watch history records (in-progress items) for the current user.
// @Tags Playback & History
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.WatchHistory
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Router /history [get]
func (h *HistoryHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	claims := apimw.ClaimsFromContext(r.Context())
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "invalid user id in token", err)
		return
	}

	items, err := sqlite.ListWatchHistory(r.Context(), h.db, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch watch history", err)
		return
	}
	if items == nil {
		items = []*models.WatchHistory{}
	}
	respondJSON(w, http.StatusOK, items)
}

// GetNowPlaying handles GET /api/v1/history/now-playing.
// Returns the most recently updated in-progress item for the authenticated user,
// bundled with its full media metadata. Responds with 204 No Content when
// nothing is currently in progress.
// @Summary Get Currently Playing Item
// @Description Returns the most recently updated in-progress item for the current user, bundled with media metadata.
// @Tags Playback & History
// @Security BearerAuth
// @Produce json
// @Success 200 {object} nowPlayingResponse
// @Success 204 "No currently playing item"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Router /history/now-playing [get]
func (h *HistoryHandler) GetNowPlaying(w http.ResponseWriter, r *http.Request) {
	claims := apimw.ClaimsFromContext(r.Context())
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "invalid user id in token", err)
		return
	}

	entry, err := sqlite.GetMostRecentHistory(r.Context(), h.db, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch now playing", err)
		return
	}
	if entry == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	media, err := sqlite.GetMediaItemByID(r.Context(), h.db, entry.MediaItemID)
	if errors.Is(err, sqlite.ErrNotFound) {
		// Media was deleted; treat as nothing playing.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	respondJSON(w, http.StatusOK, &nowPlayingResponse{History: entry, Media: media})
}

// UpsertHistory handles PUT /api/v1/history/{mediaID}.
// Creates or updates the watch position for the authenticated user and the
// given media item. Clients should call this periodically (e.g. every 10 s).
// @Summary Upsert Playback Position
// @Description Record active playback seconds and completion status for a media item.
// @Tags Playback & History
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param mediaID path string true "Media Item ID" format(uuid)
// @Param body body upsertHistoryRequest true "Active position payload"
// @Success 200 {object} models.WatchHistory
// @Failure 400 {object} map[string]string "Invalid input or body"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 404 {object} map[string]string "Media item not found"
// @Router /history/{mediaID} [put]
func (h *HistoryHandler) UpsertHistory(w http.ResponseWriter, r *http.Request) {
	claims := apimw.ClaimsFromContext(r.Context())
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "invalid user id in token", err)
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
		respondError(w, http.StatusInternalServerError, "could not verify media item", err)
		return
	}

	entry := &models.WatchHistory{
		UserID:      userID,
		MediaItemID: mediaID,
		Position:    req.Position,
		Completed:   req.Completed,
	}
	if err := sqlite.UpsertWatchHistory(r.Context(), h.db, entry); err != nil {
		respondError(w, http.StatusInternalServerError, "could not save watch history", err)
		return
	}

	respondJSON(w, http.StatusOK, entry)
}
