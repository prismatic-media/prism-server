package handler

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/ringmaster217/prism/internal/models"
	"github.com/ringmaster217/prism/internal/scanner"
	"github.com/ringmaster217/prism/internal/store/sqlite"
	"github.com/ringmaster217/prism/internal/transcoder"
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

// HandleIndex handles POST /api/v1/admin/artifacts/index.
// It scans all enabled segment storage areas for artifact sidecars and
// returns a summary of changes.
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

// HandleStatus handles GET /api/v1/admin/artifacts/status.
// It returns the current artifact health counts and unmatched artifact counts.
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

// HandleRelink handles POST /api/v1/admin/artifacts/relink.
// It performs deterministic fingerprint-based relinking between indexed
// artifacts and media items.
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

// HandleWriteSidecars handles POST /api/v1/admin/artifacts/write-sidecars.
// It iterates through all media items and writes sidecars for those that have a transcode bundle.
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
