package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/ringmaster217/prism/internal/auth"
	"github.com/ringmaster217/prism/internal/models"
	"github.com/ringmaster217/prism/internal/store/sqlite"
)

// SetupHandler handles the first-run web setup wizard.
type SetupHandler struct {
	db *sql.DB
}

func NewSetupHandler(db *sql.DB) *SetupHandler {
	return &SetupHandler{db: db}
}

type setupRequest struct {
	Username          string `json:"username"`
	Password          string `json:"password"`
	SegmentsDir       string `json:"segments_dir"`
	ThumbsDir         string `json:"thumbs_dir"`
	TMDBApiKey        string `json:"tmdb_api_key"`
	CastReceiverAppID string `json:"cast_receiver_app_id"`
}

// CompleteSetup creates the initial admin account and marks setup as complete.
// Returns 409 if setup has already been completed.
func (h *SetupHandler) CompleteSetup(w http.ResponseWriter, r *http.Request) {
	// Guard: already set up?
	done, err := sqlite.GetSetting(r.Context(), h.db, "setup_complete")
	if err != nil && err != sqlite.ErrNotFound {
		respondError(w, http.StatusInternalServerError, "could not check setup status", err)
		return
	}
	if done == "true" {
		respondError(w, http.StatusConflict, "setup already complete")
		return
	}

	var req setupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" || req.SegmentsDir == "" || req.ThumbsDir == "" {
		respondError(w, http.StatusBadRequest, "username, password, segments_dir, and thumbs_dir are required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not hash password", err)
		return
	}

	u := &models.User{
		Username:     req.Username,
		Email:        req.Username, // email not required in setup wizard
		PasswordHash: hash,
		IsAdmin:      true,
	}
	if err := sqlite.CreateUser(r.Context(), h.db, u); err != nil {
		respondError(w, http.StatusInternalServerError, "could not create admin user", err)
		return
	}

	// Save thumbs_dir setting
	if err := sqlite.SetSetting(r.Context(), h.db, "thumbs_dir", req.ThumbsDir); err != nil {
		respondError(w, http.StatusInternalServerError, "could not save thumbnails directory setting", err)
		return
	}

	// Save tmdb_api_key setting (optional)
	if err := sqlite.SetSetting(r.Context(), h.db, "tmdb_api_key", req.TMDBApiKey); err != nil {
		respondError(w, http.StatusInternalServerError, "could not save TMDB API Key setting", err)
		return
	}

	// Save cast_receiver_app_id setting (optional)
	if err := sqlite.SetSetting(r.Context(), h.db, "cast_receiver_app_id", req.CastReceiverAppID); err != nil {
		respondError(w, http.StatusInternalServerError, "could not save Chromecast Receiver App ID setting", err)
		return
	}

	// Create segment storage area
	area := &models.StorageArea{
		Kind:    models.StorageAreaKindSegments,
		Path:    req.SegmentsDir,
		Enabled: true,
	}
	if err := sqlite.CreateStorageArea(r.Context(), h.db, area); err != nil {
		respondError(w, http.StatusInternalServerError, "could not create segment storage area", err)
		return
	}

	if err := sqlite.SetSetting(context.Background(), h.db, "setup_complete", "true"); err != nil {
		respondError(w, http.StatusInternalServerError, "could not mark setup complete", err)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}
