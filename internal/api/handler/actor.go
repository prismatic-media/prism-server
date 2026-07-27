package handler

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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
		// Fallback 1: check without actor_ prefix if saved under original filename
		fallbackPath := filepath.Join(thumbsDir, filename)
		if _, err := os.Stat(fallbackPath); err == nil {
			localPath = fallbackPath
		} else {
			// Fallback 2: on-the-fly fetch from TMDB and cache locally
			if err := fetchAndSaveActorImage(r.Context(), filename, localPath); err != nil {
				respondError(w, http.StatusNotFound, "actor image file not found", err)
				return
			}
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	http.ServeFile(w, r, localPath)
}

func fetchAndSaveActorImage(ctx context.Context, filename, destPath string) error {
	cleanName := filename
	if strings.HasPrefix(cleanName, "actor_") {
		cleanName = strings.TrimPrefix(cleanName, "actor_")
	}
	if cleanName == "" {
		return fmt.Errorf("invalid filename")
	}

	imageURL := "https://image.tmdb.org/t/p/w185/" + cleanName
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("TMDB image fetch returned %d", resp.StatusCode)
	}

	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = os.Remove(destPath)
		return err
	}
	return nil
}
