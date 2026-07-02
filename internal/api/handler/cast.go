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
// @Summary Serve Cast Receiver Page
// @Description Serve the Chromecast custom HTML receiver page.
// @Tags Cast
// @Produce text/html
// @Success 200 {string} string "HTML receiver page content"
// @Router /cast-receiver [get]
// @Router /../cast-receiver [get]
func (h *CastHandler) ServeReceiver(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(receiverHTML)
}

// GetConfig handles GET /api/v1/cast-config.
// @Summary Get Cast Configuration
// @Description Retrieve the registered Google Cast App ID from settings.
// @Tags Cast
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]string "Returns {'app_id': '...'}"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /cast-config [get]
func (h *CastHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	appID, _ := sqlite.GetSetting(r.Context(), h.db, "cast_receiver_app_id")
	respondJSON(w, http.StatusOK, map[string]string{"app_id": appID})
}
