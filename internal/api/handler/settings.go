package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

// SettingsHandler handles admin server settings.
type SettingsHandler struct {
	db *sql.DB
}

func NewSettingsHandler(db *sql.DB) *SettingsHandler {
	return &SettingsHandler{db: db}
}

// GetSettings returns all user-configurable settings.
// jwt_secret and setup_complete are intentionally excluded.
// @Summary Get Server Settings (Admin Only)
// @Description Retrieve a dictionary of all editable server setting keys and values.
// @Tags Admin Configuration
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]string "Map of server configurations"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Router /admin/settings [get]
func (h *SettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := sqlite.GetAllSettings(r.Context(), h.db, true)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load settings", err)
		return
	}
	respondJSON(w, http.StatusOK, settings)
}

// UpdateSettings accepts a JSON object of setting key-value pairs and persists
// them. Only known, user-configurable keys are accepted; unknown keys return 400.
// @Summary Update Server Settings (Admin Only)
// @Description Update one or more server setting values. Unknown keys return a bad request status.
// @Tags Admin Configuration
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body map[string]string true "Settings key-value pairs"
// @Success 200 {object} map[string]string "Returns {'status': 'ok'}"
// @Failure 400 {object} map[string]string "Invalid request body or unknown configuration key"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Router /admin/settings [put]
func (h *SettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var incoming map[string]string
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	for key := range incoming {
		if !sqlite.IsConfigurableKey(key) {
			respondError(w, http.StatusBadRequest, "unknown setting key: "+key)
			return
		}
	}

	for key, value := range incoming {
		if err := sqlite.SetSetting(r.Context(), h.db, key, value); err != nil {
			respondError(w, http.StatusInternalServerError, "could not save setting: "+key, err)
			return
		}
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
