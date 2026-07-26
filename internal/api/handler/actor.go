package handler

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

// ActorHandler handles requests for actor headshots and media metadata images.
type ActorHandler struct {
	db *sql.DB
}

// NewActorHandler creates a new ActorHandler.
func NewActorHandler(db *sql.DB) *ActorHandler {
	return &ActorHandler{db: db}
}

// ServeActorImage serves a cached profile photo image for an actor.
// @Summary Serve Actor Image
// @Description Serve the cached headshot image file for an actor by filename.
// @Tags Actors
// @Produce image/*
// @Param filename path string true "Actor profile filename"
// @Success 200 {file} file "Actor image file"
// @Failure 400 {object} map[string]string "Invalid filename"
// @Failure 404 {object} map[string]string "Actor image file not found"
// @Router /actors/image/{filename} [get]
func (h *ActorHandler) ServeActorImage(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if filename == "" || strings.Contains(filename, "/") || strings.Contains(filename, "..") {
		respondError(w, http.StatusBadRequest, "invalid filename", nil)
		return
	}

	thumbsDir, err := sqlite.GetSetting(r.Context(), h.db, "thumbs_dir")
	if err != nil || thumbsDir == "" {
		respondError(w, http.StatusNotFound, "thumbs directory not configured", err)
		return
	}

	// Actor images are saved with "actor_" prefix on disk
	localPath := filepath.Join(thumbsDir, "actor_"+filename)
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		// Fallback: check without actor_ prefix if saved under original filename
		fallbackPath := filepath.Join(thumbsDir, filename)
		if _, err := os.Stat(fallbackPath); err == nil {
			localPath = fallbackPath
		} else {
			respondError(w, http.StatusNotFound, "actor image file not found", nil)
			return
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	http.ServeFile(w, r, localPath)
}
