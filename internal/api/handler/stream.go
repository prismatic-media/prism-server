package handler

import (
	"database/sql"
	"errors"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/prismatic-media/prism-server/internal/auth"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/pkg/dash"
)

// StreamHandler serves DASH manifests and fMP4 segment files.
type StreamHandler struct {
	db          *sql.DB
	mpdCache    *dash.Cache
	jwtSecret   string
}

func NewStreamHandler(db *sql.DB, mpdCache *dash.Cache, jwtSecret string) *StreamHandler {
	return &StreamHandler{db: db, mpdCache: mpdCache, jwtSecret: jwtSecret}
}

// ServeManifest handles GET /api/v1/stream/{id}/manifest.mpd.
// It looks up the mpd_path from the database (falling back to the in-process
// cache) and serves the file with the correct Content-Type.
//
// Migration safety: manifest resolution uses mpd_path from the database. The
// artifact_records table is NOT consulted during normal streaming — it is only
// used by admin operations (indexing, relinking) to repair and maintain links
// between artifacts and media items.
// @Summary Serve DASH Manifest
// @Description Retrieve the DASH master manifest file (.mpd) for a media item. Consumable by video players.
// @Tags Media Streaming
// @Security BearerAuth
// @Produce application/dash+xml
// @Param id path string true "Media ID" format(uuid)
// @Param cast_token query string false "Cast validation token"
// @Success 200 {file} file "DASH Manifest XML file"
// @Failure 400 {object} map[string]string "Invalid media ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 404 {object} map[string]string "Media item not found or transcode pending"
// @Router /stream/{id}/manifest.mpd [get]
func (h *StreamHandler) ServeManifest(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id")
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	// Prefer DB-persisted path; fall back to in-process cache.
	mpdPath := ""
	if item.MPDPath != nil {
		mpdPath = *item.MPDPath
	}
	if mpdPath == "" {
		var ok bool
		mpdPath, ok = h.mpdCache.Get(id)
		if !ok {
			respondError(w, http.StatusNotFound, "manifest not yet available — transcode pending")
			return
		}
	}

	// If no mpd_path is available, try to find a linked artifact's manifest.
	// This enables manifest resolution after database loss when artifact
	// links have been repaired via the relink operation.
	if mpdPath == "" {
		links, err := sqlite.GetArtifactMediaLinkByMedia(r.Context(), h.db, id)
		if err == nil && len(links) > 0 {
			art, err := sqlite.GetArtifactRecordByID(r.Context(), h.db, links[0].ArtifactID)
			if err == nil && art != nil && art.OutputDir != "" {
				mpdPath = filepath.Join(art.OutputDir, art.MPDPath)
			}
		}
		_ = links
	}

	w.Header().Set("Content-Type", "application/dash+xml")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, mpdPath)
}

// ServeSegment handles GET /api/v1/stream/{id}/segments/*.
// The wildcard path is resolved relative to the media item's output directory.
// Segment files get long-lived cache headers; init segments get shorter ones.
// @Summary Serve Media Segment
// @Description Retrieve a specific video/audio media segment file (.m4s, .mp4, subtitles) for adaptive streaming.
// @Tags Media Streaming
// @Security BearerAuth
// @Produce video/mp4,video/iso.segment,text/vtt,application/octet-stream
// @Param id path string true "Media ID" format(uuid)
// @Param wildcard path string true "Segment path wildcard"
// @Param cast_token query string false "Cast validation token"
// @Success 200 {file} file "Raw media segment chunk"
// @Failure 400 {object} map[string]string "Invalid parameters"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 404 {object} map[string]string "Segment not found"
// @Router /stream/{id}/segments/{wildcard} [get]
func (h *StreamHandler) ServeSegment(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id")
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	mpdPath := ""
	if item.MPDPath != nil {
		mpdPath = *item.MPDPath
	}
	if mpdPath == "" {
		var ok bool
		mpdPath, ok = h.mpdCache.Get(id)
		if !ok {
			respondError(w, http.StatusNotFound, "manifest not yet available — transcode pending")
			return
		}
	}
	outputDir := filepath.Dir(mpdPath)

	// The wildcard captures everything after .../segments/
	wildcardPath := chi.URLParam(r, "*")
	if strings.Contains(wildcardPath, "..") {
		respondError(w, http.StatusBadRequest, "invalid segment path")
		return
	}

	segPath := filepath.Join(outputDir, filepath.FromSlash(wildcardPath))

	// Set content type based on extension.
	ext := strings.ToLower(filepath.Ext(segPath))
	ct := contentTypeForSegment(ext)
	w.Header().Set("Content-Type", ct)

	// Segment files are immutable once written; init segments get shorter cache.
	if ext == ".m4s" {
		w.Header().Set("Cache-Control", "max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "max-age=3600")
	}

	// http.ServeFile handles Range requests automatically.
	http.ServeFile(w, r, segPath)
}

// IssueCastToken handles POST /api/v1/stream/{id}/cast-token.
// It issues a short-lived token that Chromecast devices can embed in the
// manifest URL as ?cast_token=... to authenticate without custom headers.
// @Summary Issue Cast Token
// @Description Generates a short-lived token to authenticate Chromecast playback URLs.
// @Tags Media Streaming
// @Security BearerAuth
// @Produce json
// @Param id path string true "Media ID" format(uuid)
// @Success 200 {object} map[string]string "Returns {'token': '...'}"
// @Failure 400 {object} map[string]string "Invalid media ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Router /stream/{id}/cast-token [post]
func (h *StreamHandler) IssueCastToken(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id")
		return
	}

	token, err := auth.IssueCastToken(h.jwtSecret, id.String())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not issue cast token", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"token": token})
}

func contentTypeForSegment(ext string) string {
	switch ext {
	case ".mp4":
		return "video/mp4"
	case ".m4s":
		return "video/iso.segment"
	case ".mpd":
		return "application/dash+xml"
	case ".vtt":
		return "text/vtt"
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}
