package handler

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/scanner"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/internal/transcoder"
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
