package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/ringmaster217/prism/internal/store/sqlite"
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
