package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/artifact"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/pkg/dash"
	"github.com/prismatic-media/prism-server/pkg/events"
	"github.com/prismatic-media/prism-server/pkg/ffmpeg"
	"github.com/prismatic-media/prism-server/pkg/subtitle"
)

// MediaHandler handles media item queries and deletions.
type MediaHandler struct {
	db  *sql.DB
	bus *events.Bus
}

func NewMediaHandler(db *sql.DB) *MediaHandler {
	return &MediaHandler{db: db}
}

func (h *MediaHandler) WithBus(bus *events.Bus) *MediaHandler {
	h.bus = bus
	return h
}

// ListMedia returns all media items. If a library_id query parameter is
// supplied, only items for that library are returned.
// @Summary List Media Items
// @Description Lists media items (movies, episodes) with filtering/sorting capabilities.
// @Tags Media Items
// @Security BearerAuth
// @Produce json
// @Param library_id query string false "Library ID" format(uuid)
// @Param media_type query string false "Media Type" enum(movie,episode)
// @Param sort query string false "Sort order" enum(recent)
// @Param limit query integer false "Recent items count limit" default(20)
// @Success 200 {array} models.MediaItem
// @Failure 400 {object} map[string]string "Invalid library ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Router /movies [get]
func (h *MediaHandler) ListMedia(w http.ResponseWriter, r *http.Request) {
	qStr := r.URL.Query().Get("q")
	if qStr != "" {
		items, err := sqlite.SearchMovies(r.Context(), h.db, qStr)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not search movies", err)
			return
		}
		respondJSON(w, http.StatusOK, emptySlice(items))
		return
	}

	sortStr := r.URL.Query().Get("sort")
	if sortStr == "recent" {
		limit := 20
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		items, err := sqlite.ListRecentMediaItems(r.Context(), h.db, limit)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not list recent media items", err)
			return
		}
		respondJSON(w, http.StatusOK, emptySlice(items))
		return
	}

	libIDStr := r.URL.Query().Get("library_id")
	allStr := r.URL.Query().Get("all")

	if libIDStr != "" {
		libID, err := uuid.Parse(libIDStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid library_id", err)
			return
		}
		var items []*models.MediaItem
		if allStr == "true" {
			items, err = sqlite.ListMediaItemsAll(r.Context(), h.db, libID)
		} else {
			items, err = sqlite.ListMediaItems(r.Context(), h.db, libID)
		}
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not list media items", err)
			return
		}
		respondJSON(w, http.StatusOK, emptySlice(items))
		return
	}

	var items []*models.MediaItem
	var err error
	if allStr == "true" {
		items, err = sqlite.ListAllMediaItemsAll(r.Context(), h.db)
	} else {
		items, err = sqlite.ListAllMediaItems(r.Context(), h.db)
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list media items", err)
		return
	}
	respondJSON(w, http.StatusOK, emptySlice(items))
}

// Search queries media items and TV shows matching a search string.
// @Summary Global Search
// @Description Searches library for shows and movies by title.
// @Tags Media Items
// @Security BearerAuth
// @Produce json
// @Param q query string true "Search query string"
// @Success 200 {array} models.SearchResult
// @Failure 401 {object} map[string]string "Unauthenticated"
// (Deprecated global search endpoint - search is now query parameter based on /movies and /tv-shows)
func (h *MediaHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		respondJSON(w, http.StatusOK, []any{})
		return
	}

	results, err := sqlite.SearchMedia(r.Context(), h.db, q)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not execute search", err)
		return
	}

	respondJSON(w, http.StatusOK, emptySlice(results))
}

// GetMedia returns a single media item by ID.
// @Summary Get Media Item
// @Tags Media Items
// @Security BearerAuth
// @Produce json
// @Param id path string true "Media ID" format(uuid)
// @Success 200 {object} models.MediaItem
// @Failure 400 {object} map[string]string "Invalid media ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 404 {object} map[string]string "Media item not found"
// @Router /movies/{id} [get]
func (h *MediaHandler) GetMedia(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id", err)
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found", err)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	respondJSON(w, http.StatusOK, item)
}

// DeleteMedia removes a media item (admin only).
// @Summary Delete Media Item (Admin Only)
// @Tags Media Items
// @Security BearerAuth
// @Param id path string true "Media ID" format(uuid)
// @Success 204 "Media item deleted successfully"
// @Failure 400 {object} map[string]string "Invalid media ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Failure 404 {object} map[string]string "Media item not found"
// @Router /movies/{id} [delete]
func (h *MediaHandler) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id", err)
		return
	}

	if err := sqlite.DeleteMediaItem(r.Context(), h.db, id); errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found", err)
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "could not delete media item", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ServePoster serves the cached poster image for a media item.
func (h *MediaHandler) ServePoster(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id", err)
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found", err)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	if item.PosterPath == nil || *item.PosterPath == "" {
		respondError(w, http.StatusNotFound, "no poster available")
		return
	}
	http.ServeFile(w, r, *item.PosterPath)
}

// ServeBackdrop serves the cached backdrop image for a media item.
func (h *MediaHandler) ServeBackdrop(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id", err)
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found", err)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	if item.BackdropPath == nil || *item.BackdropPath == "" {
		respondError(w, http.StatusNotFound, "no backdrop available")
		return
	}
	http.ServeFile(w, r, *item.BackdropPath)
}

// ServeExtraPoster serves a cached extra poster image for a media item by index.
func (h *MediaHandler) ServeExtraPoster(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id", err)
		return
	}

	indexStr := chi.URLParam(r, "index")
	index, err := strconv.Atoi(indexStr)
	if err != nil || index < 0 {
		respondError(w, http.StatusBadRequest, "invalid extra poster index", err)
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found", err)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	if len(item.ExtraPosters) == 0 || index >= len(item.ExtraPosters) {
		respondError(w, http.StatusNotFound, "extra poster index out of bounds")
		return
	}
	http.ServeFile(w, r, item.ExtraPosters[index])
}

// GetTranscodeSizes returns the size of each resolution and the total size in the transcode bundle.
// @Summary Get Media Transcode Sizes
// @Description Returns the size of each resolution subdirectory and the total size in the media item's transcode bundle.
// @Tags Media Items
// @Security BearerAuth
// @Produce json
// @Param id path string true "Media ID" format(uuid)
// @Success 200 {object} models.TranscodeSizesInfo
// @Failure 400 {object} map[string]string "Invalid media ID"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 404 {object} map[string]string "Media item not found"
// @Router /movies/{id}/transcode-sizes [get]
func (h *MediaHandler) GetTranscodeSizes(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id", err)
		return
	}

	item, err := sqlite.GetMediaItemByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found", err)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	if item.MPDPath == nil || *item.MPDPath == "" {
		respondJSON(w, http.StatusOK, models.TranscodeSizesInfo{Renditions: []models.RenditionSize{}})
		return
	}

	if item.TranscodeSizes != nil && len(item.TranscodeSizes.Renditions) > 0 {
		sortRenditions(item.TranscodeSizes.Renditions)
		respondJSON(w, http.StatusOK, item.TranscodeSizes)
		return
	}

	outputDir := filepath.Dir(*item.MPDPath)
	// Try reading from sidecar
	if meta, err := artifact.ReadSidecar(outputDir); err == nil && meta != nil && meta.Sizes != nil && len(meta.Sizes.Renditions) > 0 {
		sortRenditions(meta.Sizes.Renditions)
		// Update DB with sidecar sizes
		_ = sqlite.SetMediaTranscodeSizes(r.Context(), h.db, item.ID, meta.Sizes)
		respondJSON(w, http.StatusOK, meta.Sizes)
		return
	}

	// Fallback to dynamic scanning
	sizes := artifact.GetTranscodeSizesInfo(*item.MPDPath)
	sortRenditions(sizes.Renditions)

	// Cache to database
	_ = sqlite.SetMediaTranscodeSizes(r.Context(), h.db, item.ID, &sizes)

	// Cache to sidecar if sidecar exists
	if meta, err := artifact.ReadSidecar(outputDir); err == nil && meta != nil {
		meta.Sizes = &sizes
		_ = artifact.WriteSidecar(outputDir, meta)
	}

	respondJSON(w, http.StatusOK, sizes)
}

func sortRenditions(renditions []models.RenditionSize) {
	sort.Slice(renditions, func(i, j int) bool {
		valI := parseResolution(renditions[i].Resolution)
		valJ := parseResolution(renditions[j].Resolution)
		return valI < valJ
	})
}

func parseResolution(res string) int {
	resLower := strings.ToLower(res)
	if strings.HasPrefix(resLower, "4k") {
		return 2160
	}
	if strings.HasPrefix(resLower, "8k") {
		return 4320
	}
	var digits strings.Builder
	for _, ch := range res {
		if ch >= '0' && ch <= '9' {
			digits.WriteRune(ch)
		} else {
			break
		}
	}
	if digits.Len() > 0 {
		val, _ := strconv.Atoi(digits.String())
		return val
	}
	return 0
}

// emptySlice converts a nil slice to an empty interface slice for JSON
// serialisation — ensures the response is [] rather than null.
func emptySlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// UploadSubtitle handles POST /api/v1/media/{id}/subtitles (Admin Only).
// It accepts an SRT file, converts it to WebVTT via FFmpeg, and stores it in the database.
func (h *MediaHandler) UploadSubtitle(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "media_id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id", err)
		return
	}

	// Validate media item exists
	_, err = sqlite.GetMediaItemByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found", err)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	err = r.ParseMultipartForm(10 << 20) // max 10MB
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse multipart form", err)
		return
	}

	lang := r.FormValue("language")
	label := r.FormValue("label")
	if lang == "" || label == "" {
		respondError(w, http.StatusBadRequest, "language and label are required")
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "file is required", err)
		return
	}
	defer func() { _ = file.Close() }()

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".srt" {
		respondError(w, http.StatusBadRequest, "only SRT files are supported")
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read uploaded file", err)
		return
	}

	vttBytes, err := ffmpeg.ConvertSRTToVTT(r.Context(), "ffmpeg", fileBytes)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to convert SRT to WebVTT: "+err.Error(), err)
		return
	}

	sub := &models.MediaSubtitle{
		MediaItemID: id,
		Language:    lang,
		Label:       label,
		VTTContent:  string(vttBytes),
	}

	err = sqlite.AddMediaSubtitle(r.Context(), h.db, sub)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save subtitle to database", err)
		return
	}

	// Trigger similarity matching and sync auto-alignment in the background only if Whisper transcription exists
	hasWhisper, err := sqlite.HasWhisperTranscription(r.Context(), h.db, sub.MediaItemID)
	if err == nil && hasWhisper {
		go RunSubtitleAlignment(context.Background(), h.db, h.bus, sub.ID)
	} else {
		_ = sqlite.UpdateMediaSubtitleStatus(r.Context(), h.db, sub.ID, "pending")
		sub.AlignmentStatus = "pending"
	}

	respondJSON(w, http.StatusCreated, sub)
}

// SyncSubtitleRequest is the payload for manual shift or auto-sync trigger.
type SyncSubtitleRequest struct {
	Offset *float64 `json:"offset,omitempty"`
}

// SyncSubtitle handles POST /api/v1/media/subtitles/{id}/sync (Admin Only).
// It can trigger automatic alignment or apply a manual timestamp shift.
func (h *MediaHandler) SyncSubtitle(w http.ResponseWriter, r *http.Request) {
	mediaID, err := uuidParam(r, "media_id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id", err)
		return
	}
	id, err := uuidParam(r, "subtitle_id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid subtitle id", err)
		return
	}

	sub, err := sqlite.GetMediaSubtitleByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "subtitle not found", err)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch subtitle", err)
		return
	}
	if sub.MediaItemID != mediaID {
		respondError(w, http.StatusBadRequest, "subtitle does not belong to specified media item")
		return
	}

	var req SyncSubtitleRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid JSON payload", err)
			return
		}
	}

	if req.Offset != nil {
		// Manual shift
		offsetVal := *req.Offset
		shiftedVTT, err := subtitle.ShiftVTT(sub.VTTContent, offsetVal)
		if err != nil {
			respondError(w, http.StatusBadRequest, "failed to shift VTT timestamps: "+err.Error(), err)
			return
		}

		newOffset := sub.SyncOffset + offsetVal
		status := "completed"
		err = sqlite.UpdateMediaSubtitleAlignment(r.Context(), h.db, sub.ID, status, sub.SimilarityScore, newOffset, shiftedVTT)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to update subtitle alignment in database", err)
			return
		}

		err = syncSubtitlesAndRegenerateMPD(r.Context(), h.db, sub.MediaItemID)
		if err != nil {
			slog.Warn("syncing subtitles failed after manual shift", "subtitle_id", sub.ID, "error", err)
		}

		// Update subtitle struct for JSON response
		sub.VTTContent = shiftedVTT
		sub.SyncOffset = newOffset
		sub.AlignmentStatus = status

		h.bus.Publish(events.EventSubtitleAligned, events.SubtitleAlignedPayload{
			SubtitleID:      sub.ID,
			MediaItemID:     sub.MediaItemID,
			SimilarityScore: sub.SimilarityScore,
			SyncOffset:      newOffset,
			AlignmentStatus: status,
		})

		respondJSON(w, http.StatusOK, sub)
		return
	}

	// Auto sync fallback
	hasWhisper, err := sqlite.HasWhisperTranscription(r.Context(), h.db, sub.MediaItemID)
	if err != nil || !hasWhisper {
		respondError(w, http.StatusBadRequest, "cannot auto-sync: Whisper transcription is not available for this media item yet.")
		return
	}

	sub.AlignmentStatus = "processing"
	err = sqlite.UpdateMediaSubtitleStatus(r.Context(), h.db, sub.ID, "processing")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update subtitle status", err)
		return
	}

	go RunSubtitleAlignment(context.Background(), h.db, h.bus, sub.ID)

	respondJSON(w, http.StatusAccepted, sub)
}

// ListSubtitles handles GET /api/v1/media/{id}/subtitles.
func (h *MediaHandler) ListSubtitles(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "media_id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id", err)
		return
	}

	// Validate media item exists
	_, err = sqlite.GetMediaItemByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "media item not found", err)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch media item", err)
		return
	}

	subs, err := sqlite.ListMediaSubtitles(r.Context(), h.db, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list subtitles", err)
		return
	}

	respondJSON(w, http.StatusOK, emptySlice(subs))
}

// DeleteSubtitle handles DELETE /api/v1/media/subtitles/{id} (Admin Only).
func (h *MediaHandler) DeleteSubtitle(w http.ResponseWriter, r *http.Request) {
	mediaID, err := uuidParam(r, "media_id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid media id", err)
		return
	}
	id, err := uuidParam(r, "subtitle_id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid subtitle id", err)
		return
	}

	sub, err := sqlite.GetMediaSubtitleByID(r.Context(), h.db, id)
	if errors.Is(err, sqlite.ErrNotFound) {
		respondError(w, http.StatusNotFound, "subtitle not found", err)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not fetch subtitle", err)
		return
	}
	if sub.MediaItemID != mediaID {
		respondError(w, http.StatusBadRequest, "subtitle does not belong to specified media item")
		return
	}

	err = sqlite.DeleteMediaSubtitle(r.Context(), h.db, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete subtitle from database", err)
		return
	}

	// Sync to output dir and regenerate manifest
	err = syncSubtitlesAndRegenerateMPD(r.Context(), h.db, sub.MediaItemID)
	if err != nil {
		fmt.Printf("warning: syncing subtitles failed after deletion: %v\n", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

func syncSubtitlesAndRegenerateMPD(ctx context.Context, db *sql.DB, mediaID uuid.UUID) error {
	item, err := sqlite.GetMediaItemByID(ctx, db, mediaID)
	if err != nil {
		return fmt.Errorf("getting media item: %w", err)
	}

	if item.MPDPath == nil || *item.MPDPath == "" {
		return nil
	}

	mpdPath := *item.MPDPath
	outputDir := filepath.Dir(mpdPath)

	uploadedSubs, err := sqlite.ListMediaSubtitles(ctx, db, mediaID)
	if err != nil {
		return fmt.Errorf("listing database subtitles: %w", err)
	}

	// Remove old uploaded VTT files
	entries, err := os.ReadDir(outputDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "sub_uploaded_") && strings.HasSuffix(entry.Name(), ".vtt") {
				_ = os.Remove(filepath.Join(outputDir, entry.Name()))
			}
		}
	}

	// Write current database subtitles to outputDir
	for _, sub := range uploadedSubs {
		filename := fmt.Sprintf("sub_uploaded_%s_%s.vtt", sub.Language, sub.ID.String())
		subPath := filepath.Join(outputDir, filename)
		if err := os.WriteFile(subPath, []byte(sub.VTTContent), 0o644); err != nil {
			return fmt.Errorf("writing subtitle file %s: %w", filename, err)
		}
	}

	return regenerateSingleMPD(ctx, db, item)
}

func regenerateSingleMPD(ctx context.Context, db *sql.DB, item *models.MediaItem) error {
	if item.MPDPath == nil || *item.MPDPath == "" {
		return nil
	}
	mpdPath := *item.MPDPath
	outputDir := filepath.Dir(mpdPath)

	var renditions []dash.RenditionInfo
	subJobs, err := sqlite.GetMediaItemLatestJobSubJobs(ctx, db, item.ID)
	if err == nil && len(subJobs) > 0 {
		for _, sj := range subJobs {
			if sj.Type == models.SubJobTypeVideo && sj.Status == models.TranscodeStatusDone {
				name := ""
				if sj.ProfileName != nil {
					name = *sj.ProfileName
				}
				width := 0
				if sj.Width != nil {
					width = *sj.Width
				}
				height := 0
				if sj.Height != nil {
					height = *sj.Height
				}
				videoBitrateK := 0
				if sj.VideoBitrateK != nil {
					videoBitrateK = *sj.VideoBitrateK
				}
				audioBitrateK := 0
				if sj.AudioBitrateK != nil {
					audioBitrateK = *sj.AudioBitrateK
				}
				codec := "h264"
				if sj.Codec != nil {
					codec = *sj.Codec
				}
				renditions = append(renditions, dash.RenditionInfo{
					Name:          name,
					Width:         width,
					Height:        height,
					VideoBitrateK: videoBitrateK,
					AudioBitrateK: audioBitrateK,
					Codec:         codec,
				})
			}
		}
	}

	if len(renditions) == 0 {
		if meta, err := artifact.ReadSidecar(outputDir); err == nil && meta != nil && len(meta.Profiles) > 0 {
			for _, p := range meta.Profiles {
				renditions = append(renditions, dash.RenditionInfo{
					Name:          p.Name,
					Width:         p.Width,
					Height:        p.Height,
					VideoBitrateK: p.VideoBitrateK,
					AudioBitrateK: p.AudioBitrateK,
					Codec:         "h264",
				})
			}
		}
	}

	if len(renditions) == 0 {
		return fmt.Errorf("no profile or sub-job information found to regenerate the MPD")
	}

	var subs []dash.SubtitleInfo
	entries, err := os.ReadDir(outputDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "sub_") && strings.HasSuffix(entry.Name(), ".vtt") {
				lang := strings.TrimPrefix(entry.Name(), "sub_")
				lang = strings.TrimSuffix(lang, ".vtt")
				subs = append(subs, dash.SubtitleInfo{
					Language: lang,
					VTTPath:  filepath.Join(outputDir, entry.Name()),
				})
			}
		}
	}

	return dash.GenerateMPD(outputDir, mpdPath, renditions, subs, item.Duration)
}
