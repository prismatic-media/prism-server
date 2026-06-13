package handler

import (
	_ "embed"
	"database/sql"
	"net/http"

	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

//go:embed receiver.html
var receiverHTML []byte

// CastHandler serves the Chromecast custom receiver page and the
// App ID configuration endpoint consumed by the Angular sender.
// Rebuild triggered for updated receiver HTML template with blurred backdrop.
type CastHandler struct {
	db *sql.DB
}

func NewCastHandler(db *sql.DB) *CastHandler {
	return &CastHandler{db: db}
}

// ServeReceiver handles GET /cast-receiver.
// This URL is registered in the Google Cast Developer Console as the
// receiver application URL. The Chromecast device fetches it directly.
func (h *CastHandler) ServeReceiver(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(receiverHTML)
}

// GetConfig handles GET /api/v1/cast/config.
// Returns the Cast App ID so the Angular sender doesn't need it hardcoded.
func (h *CastHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	appID, _ := sqlite.GetSetting(r.Context(), h.db, "cast_receiver_app_id")
	respondJSON(w, http.StatusOK, map[string]string{"app_id": appID})
}
