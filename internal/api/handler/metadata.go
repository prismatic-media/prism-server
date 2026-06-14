package handler

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/prismatic-media/prism-server/internal/metadata"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

// MetadataHandler exposes admin endpoints for bulk metadata operations.
type MetadataHandler struct {
	db       *sql.DB
	enricher *metadata.Enricher
}

func NewMetadataHandler(db *sql.DB, enricher *metadata.Enricher) *MetadataHandler {
	return &MetadataHandler{db: db, enricher: enricher}
}

// RefreshAllMetadata clears every TMDB field (tmdb_id, year, overview,
// poster_path) across media_items, tv_shows, and tv_seasons, then re-runs the
// enricher on every item in a background goroutine.
//
// The response is immediate (202 Accepted) with a count of affected rows.
// @Summary Refresh All Metadata
// @Description Clear metadata across all media items, TV shows, and TV seasons, and re-trigger bulk background TMDB metadata enrichment.
// @Tags Metadata Administration
// @Security BearerAuth
// @Produce json
// @Success 202 {object} map[string]any "Returns status and count of cleared records"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /admin/metadata/refresh [post]
func (h *MetadataHandler) RefreshAllMetadata(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Clear metadata on all three tables.
	count, err := sqlite.ClearAllMediaMetadata(ctx, h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to clear media metadata", err)
		return
	}
	if err := sqlite.ClearAllTVShowMetadata(ctx, h.db); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to clear tv show metadata", err)
		return
	}
	if err := sqlite.ClearAllTVSeasonMetadata(ctx, h.db); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to clear tv season metadata", err)
		return
	}

	// 2. Load all items (including episodes) for background enrichment.
	items, err := sqlite.ListAllMediaItemsAll(ctx, h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list media items", err)
		return
	}

	// 3. Respond immediately, then run enrichment in the background.
	respondJSON(w, http.StatusAccepted, map[string]any{
		"status": "refresh started",
		"count":  count,
	})

	enricher := h.enricher
	go func() {
		bgCtx := context.Background()
		for _, item := range items {
			switch item.MediaType {
			case models.MediaTypeEpisode:
				if item.TVShowID != nil && item.TVSeasonID != nil {
					enricher.EnrichTVEpisode(bgCtx, item, *item.TVShowID, *item.TVSeasonID)
				}
			default:
				enricher.EnrichItem(bgCtx, item)
			}
		}
		slog.Info("metadata refresh complete", "items", len(items))
	}()
}
