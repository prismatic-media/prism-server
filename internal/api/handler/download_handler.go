package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

// DownloadHandler handles requests for offline sync and download queries.
type DownloadHandler struct {
	db *sql.DB
}

// NewDownloadHandler creates a new DownloadHandler.
func NewDownloadHandler(db *sql.DB) *DownloadHandler {
	return &DownloadHandler{db: db}
}

// SegmentInfo represents a single segment file's relative path and size.
type SegmentInfo struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// SubtitleInfo represents a subtitle file's relative path, metadata, and size.
type SubtitleInfo struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	Label    string `json:"label"`
	Size     int64  `json:"size"`
}

// RenditionListingResponse contains all download details for a media item's quality.
type RenditionListingResponse struct {
	MediaID   string         `json:"media_id"`
	Quality   string         `json:"quality"`
	Duration  float64        `json:"duration"`
	TotalSize int64          `json:"total_size"`
	Segments  []SegmentInfo  `json:"segments"`
	Subtitles []SubtitleInfo `json:"subtitles"`
}

// ListRenditionSegments handles GET /api/v1/stream/{media_id}/renditions/{quality}.
// It scans the media item's output folder for segments matching the specified quality,
// as well as any associated WebVTT subtitle files.
// @Summary List Rendition Segments for Download
// @Description Retrieve a checklist of individual segment paths and subtitle files for a specific rendition quality to download offline.
// @Tags Media Streaming
// @Security BearerAuth
// @Produce json
// @Param media_id path string true "Media ID" format(uuid)
// @Param quality path string true "Rendition Quality"
// @Success 200 {object} RenditionListingResponse "Success"
// @Failure 400 {object} map[string]string "Invalid media ID or parameters"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 404 {object} map[string]string "Media item or rendition quality not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /stream/{media_id}/renditions/{quality} [get]
func (h *DownloadHandler) ListRenditionSegments(w http.ResponseWriter, r *http.Request) {
	mediaID, err := uuidParam(r, "media_id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id", err)
		return
	}

	quality := chi.URLParam(r, "quality")
	if quality == "" {
		respondError(w, http.StatusBadRequest, "quality parameter is required")
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, mediaID)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found", err)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	if item.MPDPath == nil || *item.MPDPath == "" {
		respondError(w, http.StatusNotFound, "manifest not yet available — transcode pending")
		return
	}

	outputDir := filepath.Dir(*item.MPDPath)
	qualityDir := filepath.Join(outputDir, quality)

	if _, err := os.Stat(qualityDir); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, fmt.Sprintf("rendition quality %q not found", quality))
		return
	}

	entries, err := os.ReadDir(qualityDir)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read quality directory", err)
		return
	}

	segments := []SegmentInfo{}
	var totalSize int64

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		segments = append(segments, SegmentInfo{
			Path: quality + "/" + entry.Name(),
			Size: info.Size(),
		})
		totalSize += info.Size()
	}

	subtitles := []SubtitleInfo{}
	uploadedSubs, _ := sqlite.ListMediaSubtitles(r.Context(), h.db, item.ID)

	parentEntries, err := os.ReadDir(outputDir)
	if err == nil {
		for _, entry := range parentEntries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "sub_") && strings.HasSuffix(entry.Name(), ".vtt") {
				filename := entry.Name()
				lang := strings.TrimPrefix(filename, "sub_")
				lang = strings.TrimSuffix(lang, ".vtt")
				label := ""

				if strings.HasPrefix(lang, "uploaded_") {
					parts := strings.Split(lang, "_")
					if len(parts) >= 3 {
						uuidStr := parts[len(parts)-1]
						for _, us := range uploadedSubs {
							if us.ID.String() == uuidStr {
								lang = us.Language
								label = us.Label
								break
							}
						}
					}
				}

				if label == "" {
					if lang == "eng" || lang == "en" {
						label = "English"
					} else if lang == "spa" || lang == "es" {
						label = "Spanish"
					} else if strings.HasPrefix(lang, "whisper_") {
						label = fmt.Sprintf("Auto-generated (%s)", strings.TrimPrefix(lang, "whisper_"))
						lang = strings.TrimPrefix(lang, "whisper_")
					} else {
						label = strings.ToUpper(lang)
					}
				}

				info, err := entry.Info()
				var size int64
				if err == nil {
					size = info.Size()
				}

				subtitles = append(subtitles, SubtitleInfo{
					Path:     filename,
					Language: lang,
					Label:    label,
					Size:     size,
				})
				totalSize += size
			}
		}
	}

	respondJSON(w, http.StatusOK, RenditionListingResponse{
		MediaID:   item.ID.String(),
		Quality:   quality,
		Duration:  item.Duration,
		TotalSize: totalSize,
		Segments:  segments,
		Subtitles: subtitles,
	})
}
