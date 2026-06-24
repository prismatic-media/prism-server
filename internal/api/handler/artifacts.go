package handler

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/prismatic-media/prism-server/internal/artifact"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/scanner"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/internal/transcoder"
	"github.com/prismatic-media/prism-server/pkg/dash"
)

// ArtifactHandler serves admin endpoints for artifact indexing and relinking.
type ArtifactHandler struct {
	db      *sql.DB
	indexer *scanner.Indexer
}

// NewArtifactHandler creates a new ArtifactHandler.
func NewArtifactHandler(db *sql.DB, indexer *scanner.Indexer) *ArtifactHandler {
	return &ArtifactHandler{
		db:      db,
		indexer: indexer,
	}
}

// IndexResponse is the JSON response for the indexing operation.
type IndexResponse struct {
	Summaries []IndexSummaryResponse `json:"summaries"`
	Total     int                    `json:"total"`
}

// IndexSummaryResponse is a per-storage-area summary.
type IndexSummaryResponse struct {
	StorageAreaID     string `json:"storage_area_id"`
	StorageAreaPath   string `json:"storage_area_path"`
	Registered        int    `json:"registered"`
	Updated           int    `json:"updated"`
	Removed           int    `json:"removed"`
	Errors            int    `json:"errors"`
	MediaItemsCreated int    `json:"media_items_created"`
}

// @Summary Index Artifacts
// @Description Scan all enabled segment storage areas for artifact artifact.json sidecar files to index them in the database.
// @Tags Artifact Administration
// @Security BearerAuth
// @Produce json
// @Success 200 {object} IndexResponse
// @Failure 409 {object} map[string]string "Conflict: Artifact schema not applied"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/artifacts/index [post]
func (h *ArtifactHandler) HandleIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	summaries, err := h.indexer.IndexAll(ctx)
	if err != nil {
		if err == sqlite.ErrArtifactSchemaNotReady {
			respondJSON(w, http.StatusConflict, map[string]string{
				"error": "artifact schema not yet applied — run goose up first",
			})
			return
		}
		respondError(w, http.StatusInternalServerError, "indexing failed: "+err.Error(), err)
		return
	}

	resp := IndexResponse{
		Summaries: make([]IndexSummaryResponse, len(summaries)),
	}
	for i, s := range summaries {
		resp.Summaries[i] = IndexSummaryResponse{
			StorageAreaID:     s.StorageAreaID.String(),
			StorageAreaPath:   s.StorageAreaPath,
			Registered:        s.Registered,
			Updated:           s.Updated,
			Removed:           s.Removed,
			Errors:            s.Errors,
			MediaItemsCreated: s.MediaItemsCreated,
		}
		resp.Total += s.Registered + s.Updated
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"summaries": resp.Summaries,
		"total":     resp.Total,
	})
}

// @Summary Get Artifact Status
// @Description Retrieve current health, unmatched, and ambiguous counts of indexed transcode artifacts across all storage areas.
// @Tags Artifact Administration
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]any "Returns active artifact statuses, counts, and health statistics"
// @Failure 409 {object} map[string]any "Conflict: Artifact schema not applied"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/artifacts/status [get]
func (h *ArtifactHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	// Check if artifact schema is ready.
	ready, err := sqlite.ArtifactSchemaReady(ctx, h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "checking artifact schema: "+err.Error(), err)
		return
	}

	if !ready {
		respondJSON(w, http.StatusConflict, map[string]any{
			"ready": false,
			"error": "artifact schema not yet applied — run goose up first",
		})
		return
	}

	// Get health counts per storage area.
	areas, err := sqlite.ListStorageAreasByKind(ctx, h.db, models.StorageAreaKindSegments, true)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "listing storage areas: "+err.Error(), err)
		return
	}

	type AreaHealthCount struct {
		Health string `json:"health"`
		Count  int    `json:"count"`
	}

	type AreaStatus struct {
		StorageAreaID   string            `json:"storage_area_id"`
		StorageAreaPath string            `json:"storage_area_path"`
		Enabled         bool              `json:"enabled"`
		ByHealth        []AreaHealthCount `json:"by_health"`
	}

	var statuses []AreaStatus
	for _, area := range areas {
		byHealth, err := sqlite.CountArtifactRecordsByHealth(ctx, h.db, area.ID)
		if err != nil {
			continue
		}
		var healthCounts []AreaHealthCount
		for _, ah := range byHealth {
			healthCounts = append(healthCounts, AreaHealthCount{
				Health: string(ah.Health),
				Count:  ah.Count,
			})
		}
		statuses = append(statuses, AreaStatus{
			StorageAreaID:   area.ID.String(),
			StorageAreaPath: area.Path,
			Enabled:         area.Enabled,
			ByHealth:        healthCounts,
		})
	}

	// Get unmatched/ambiguous counts.
	unmatched, err := sqlite.CountUnmatchedArtifacts(ctx, h.db)
	if err != nil {
		unmatched = 0
	}
	ambiguous, err := sqlite.CountAmbiguousArtifacts(ctx, h.db)
	if err != nil {
		ambiguous = 0
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"ready":     true,
		"areas":     statuses,
		"unmatched": unmatched,
		"ambiguous": ambiguous,
	})
}

// @Summary Relink Artifacts
// @Description Perform deterministic fingerprint-based matching to link indexed transcode artifacts with existing media items in the database.
// @Tags Artifact Administration
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]int "Returns link execution counts (linked, unmatched, ambiguous, invalid, skipped)"
// @Failure 409 {object} map[string]string "Conflict: Artifact schema not applied"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/artifacts/relink [post]
func (h *ArtifactHandler) HandleRelink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	result, err := h.indexer.RelinkExact(ctx)
	if err != nil {
		if err == sqlite.ErrArtifactSchemaNotReady {
			respondJSON(w, http.StatusConflict, map[string]string{
				"error": "artifact schema not yet applied — run goose up first",
			})
			return
		}
		respondError(w, http.StatusInternalServerError, "relinking failed: "+err.Error(), err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"linked":    result.Linked,
		"unmatched": result.Unmatched,
		"ambiguous": result.Ambiguous,
		"invalid":   result.Invalid,
		"skipped":   result.Skipped,
	})
}

// @Summary Write Sidecar Files
// @Description Iterate through all transcode-completed media items and generate artifact.json recovery sidecars on disk.
// @Tags Artifact Administration
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]int "Returns write execution counts (written, skipped, errors)"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/artifacts/write-sidecars [post]
func (h *ArtifactHandler) HandleWriteSidecars(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	items, err := sqlite.ListAllMediaItems(ctx, h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list media items: "+err.Error(), err)
		return
	}

	var written, skipped, errorsCount int
	for _, item := range items {
		if item.MPDPath == nil || *item.MPDPath == "" {
			skipped++
			continue
		}

		if err := transcoder.WriteSidecarForMediaItem(ctx, h.db, item.ID); err != nil {
			errorsCount++
			continue
		}
		written++
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"written": written,
		"skipped": skipped,
		"errors":  errorsCount,
	})
}

// @Summary Regenerate All MPDs (Admin Only)
// @Description Regenerate all MPEG-DASH manifest (.mpd) files on the server using database records and artifact sidecars.
// @Tags Artifact Administration
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]int "Returns regeneration counts (regenerated, errors)"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/artifacts/regenerate-mpds [post]
func (h *ArtifactHandler) HandleRegenerateMPDs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	items, err := sqlite.ListAllMediaItemsAll(ctx, h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list media items: "+err.Error(), err)
		return
	}

	var regenerated, errorsCount int
	for _, item := range items {
		if item.MPDPath == nil || *item.MPDPath == "" {
			continue
		}

		mpdPath := *item.MPDPath
		outputDir := filepath.Dir(mpdPath)

		// 1. Try to get renditions from subjobs.
		var renditions []dash.RenditionInfo
		subJobs, err := sqlite.GetMediaItemLatestJobSubJobs(ctx, h.db, item.ID)
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

		// 2. If sub-jobs were not found/empty, fall back to artifact sidecar.
		if len(renditions) == 0 {
			if meta, err := artifact.ReadSidecar(outputDir); err == nil && meta != nil && len(meta.Profiles) > 0 {
				for _, p := range meta.Profiles {
					renditions = append(renditions, dash.RenditionInfo{
						Name:          p.Name,
						Width:         p.Width,
						Height:        p.Height,
						VideoBitrateK: p.VideoBitrateK,
						AudioBitrateK: p.AudioBitrateK,
						Codec:         "h264", // default
					})
				}
			}
		}

		if len(renditions) == 0 {
			// No profile or subjob information found to regenerate the MPD
			errorsCount++
			continue
		}

		// 3. Scan outputDir for subtitle files (sub_*.vtt)
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

		// 4. Generate the MPD
		if err := dash.GenerateMPD(outputDir, mpdPath, renditions, subs, item.Duration); err != nil {
			errorsCount++
			continue
		}
		regenerated++
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"regenerated": regenerated,
		"errors":      errorsCount,
	})
}
