package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

type ProfilesHandler struct {
	db *sql.DB
}

func NewProfilesHandler(db *sql.DB) *ProfilesHandler {
	return &ProfilesHandler{db: db}
}

// List profiles
// @Summary List Transcode Profiles (Admin Only)
// @Description Retrieve a list of all defined transcode profiles.
// @Tags Admin Configuration
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.TranscodeProfile
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Router /admin/transcode-profiles [get]
func (h *ProfilesHandler) List(w http.ResponseWriter, r *http.Request) {
	profiles, err := sqlite.ListTranscodeProfiles(r.Context(), h.db, false)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list transcode profiles", err)
		return
	}
	respondJSON(w, http.StatusOK, profiles)
}

// Create profile
// @Summary Create Transcode Profile (Admin Only)
// @Description Create a new transcode profile.
// @Tags Admin Configuration
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body models.TranscodeProfile true "Profile details"
// @Success 200 {object} models.TranscodeProfile
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/transcode-profiles [post]
func (h *ProfilesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var p models.TranscodeProfile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if p.Name == "" || p.Width <= 0 || p.Height <= 0 || p.VideoBitrateK <= 0 || p.AudioBitrateK <= 0 {
		respondError(w, http.StatusBadRequest, "invalid profile parameters")
		return
	}

	if p.Codec != "h264" && p.Codec != "hevc" && p.Codec != "av1" {
		p.Codec = "h264"
	}

	if err := sqlite.CreateTranscodeProfile(r.Context(), h.db, &p); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create transcode profile", err)
		return
	}

	respondJSON(w, http.StatusOK, p)
}

// Update profile
// @Summary Update Transcode Profile (Admin Only)
// @Description Update an existing transcode profile.
// @Tags Admin Configuration
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Profile ID" format(uuid)
// @Param body body models.TranscodeProfile true "Updated profile details"
// @Success 200 {object} models.TranscodeProfile
// @Failure 400 {object} map[string]string "Invalid request body or path mismatch"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden"
// @Failure 404 {object} map[string]string "Profile not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/transcode-profiles/{id} [put]
func (h *ProfilesHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid profile id", err)
		return
	}

	var p models.TranscodeProfile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	p.ID = id

	if p.Name == "" || p.Width <= 0 || p.Height <= 0 || p.VideoBitrateK <= 0 || p.AudioBitrateK <= 0 {
		respondError(w, http.StatusBadRequest, "invalid profile parameters")
		return
	}

	if p.Codec != "h264" && p.Codec != "hevc" && p.Codec != "av1" {
		p.Codec = "h264"
	}

	if err := sqlite.UpdateTranscodeProfile(r.Context(), h.db, &p); err != nil {
		if errors.Is(err, sqlite.ErrNotFound) {
			respondError(w, http.StatusNotFound, "profile not found", err)
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to update transcode profile", err)
		return
	}

	respondJSON(w, http.StatusOK, p)
}

// Delete profile
// @Summary Delete Transcode Profile (Admin Only)
// @Description Delete a transcode profile.
// @Tags Admin Configuration
// @Security BearerAuth
// @Produce json
// @Param id path string true "Profile ID" format(uuid)
// @Success 200 {object} map[string]string "Returns {'status': 'ok'}"
// @Failure 400 {object} map[string]string "Invalid profile ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden"
// @Failure 404 {object} map[string]string "Profile not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/transcode-profiles/{id} [delete]
func (h *ProfilesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid profile id", err)
		return
	}

	if err := sqlite.DeleteTranscodeProfile(r.Context(), h.db, id); err != nil {
		if errors.Is(err, sqlite.ErrNotFound) {
			respondError(w, http.StatusNotFound, "profile not found", err)
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to delete transcode profile", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
