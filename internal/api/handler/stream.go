package handler

import (
	"database/sql"
	"errors"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ringmaster217/galactic-media-server/internal/store/sqlite"
	"github.com/ringmaster217/galactic-media-server/pkg/dash"
)

// StreamHandler serves DASH manifests and fMP4 segment files.
type StreamHandler struct {
	db          *sql.DB
	segmentsDir string
	mpdCache    *dash.Cache
}

func NewStreamHandler(db *sql.DB, segmentsDir string, mpdCache *dash.Cache) *StreamHandler {
	return &StreamHandler{db: db, segmentsDir: segmentsDir, mpdCache: mpdCache}
}

// ServeManifest handles GET /api/v1/stream/{id}/manifest.mpd.
// It looks up the mpd_path from the database (falling back to the in-process
// cache) and serves the file with the correct Content-Type.
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
		respondError(w, http.StatusInternalServerError, "could not fetch media item")
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

	w.Header().Set("Content-Type", "application/dash+xml")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, mpdPath)
}

// ServeSegment handles GET /api/v1/stream/{id}/segments/*.
// The wildcard path is resolved relative to the media item's output directory.
// Segment files get long-lived cache headers; init segments get shorter ones.
func (h *StreamHandler) ServeSegment(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id")
		return
	}

	// The wildcard captures everything after .../segments/
	wildcardPath := chi.URLParam(r, "*")
	if strings.Contains(wildcardPath, "..") {
		respondError(w, http.StatusBadRequest, "invalid segment path")
		return
	}

	segPath := filepath.Join(h.segmentsDir, id.String(), filepath.FromSlash(wildcardPath))

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
