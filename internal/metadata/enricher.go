package metadata

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/ringmaster217/galactic-media-server/internal/models"
	"github.com/ringmaster217/galactic-media-server/internal/store/sqlite"
)

// Enricher fetches TMDB metadata for a MediaItem and persists it to the DB.
// It is best-effort: all errors are logged rather than propagated.
type Enricher struct {
	db        *sql.DB
	client    *Client
	thumbsDir string
}

// NewEnricher creates an Enricher. When apiKey is empty, EnrichItem is a
// no-op (safe to call unconditionally even when TMDB is not configured).
func NewEnricher(db *sql.DB, apiKey, thumbsDir string) *Enricher {
	var c *Client
	if apiKey != "" {
		c = NewClient(apiKey)
	}
	return &Enricher{db: db, client: c, thumbsDir: thumbsDir}
}

// EnrichItem looks up TMDB metadata for item and writes it to the database.
// It is a no-op when item already has a TMDB ID or when no API key is
// configured.
func (e *Enricher) EnrichItem(ctx context.Context, item *models.MediaItem) {
	if e.client == nil || item.TMDBId != nil {
		return
	}

	title, year := ParseTitle(item.FilePath)

	var (
		result *TMDBResult
		err    error
	)
	switch item.MediaType {
	case models.MediaTypeMovie:
		result, err = e.client.SearchMovie(ctx, title, year)
	case models.MediaTypeTVShow, models.MediaTypeEpisode:
		result, err = e.client.SearchTV(ctx, title)
	default:
		// Music and unknown types are not enriched via TMDB.
		return
	}
	if err != nil {
		slog.Warn("TMDB search failed", "title", title, "error", err)
		return
	}
	if result == nil {
		slog.Debug("no TMDB match found", "title", title)
		return
	}

	// Download poster image when thumbs directory is configured.
	localPoster := ""
	if e.thumbsDir != "" && result.PosterPath != "" {
		localPoster, err = e.client.DownloadPoster(ctx, result.PosterPath, e.thumbsDir)
		if err != nil {
			slog.Warn("poster download failed", "title", title, "error", err)
			// Continue — we still persist the rest of the metadata.
		}
	}

	if err := sqlite.UpdateMediaMetadata(
		ctx, e.db, item.ID,
		result.ID, result.Year, result.Overview, localPoster,
	); err != nil {
		slog.Warn("storing TMDB metadata failed", "id", item.ID, "error", err)
	}
}
