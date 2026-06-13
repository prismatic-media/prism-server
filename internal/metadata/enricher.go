package metadata

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

// Enricher fetches TMDB metadata for a MediaItem and persists it to the DB.
// It is best-effort: all errors are logged rather than propagated.
// The TMDB API key and thumbnails directory are read from the settings table on
// each call so that changes take effect immediately without a server restart.
type Enricher struct {
	db              *sql.DB
	baseURLOverride string // non-empty in tests to redirect TMDB calls
}

// NewEnricher creates an Enricher. When no TMDB API key is configured in the
// database, all Enrich* methods are no-ops (safe to call unconditionally).
func NewEnricher(db *sql.DB) *Enricher {
	return &Enricher{db: db}
}

// client returns a TMDB client configured with the current API key, or nil if
// no key is set. A nil client means enrichment is disabled.
func (e *Enricher) client(ctx context.Context) *Client {
	apiKey, _ := sqlite.GetSetting(ctx, e.db, "tmdb_api_key")
	if apiKey == "" {
		return nil
	}
	c := NewClient(apiKey)
	if e.baseURLOverride != "" {
		c.baseURL = e.baseURLOverride
	}
	return c
}

// thumbsDir returns the current thumbnails directory from settings.
func (e *Enricher) thumbsDir(ctx context.Context) string {
	dir, _ := sqlite.GetSetting(ctx, e.db, "thumbs_dir")
	return dir
}

// EnrichItem looks up TMDB metadata for a movie item and writes it to the
// database. It is a no-op when the item already has a TMDB ID, when no API
// key is configured, or when the item is a TV episode (use EnrichTVEpisode).
func (e *Enricher) EnrichItem(ctx context.Context, item *models.MediaItem) {
	c := e.client(ctx)
	if c == nil {
		slog.Info("skipping enrichment: no TMDB API key configured", "id", item.ID, "title", item.Title)
		return
	}
	if item.TMDBId != nil {
		return
	}
	if item.MediaType == models.MediaTypeEpisode {
		return
	}

	title, year := ParseTitle(item.FilePath)

	var (
		result *TMDBResult
		err    error
	)
	switch item.MediaType {
	case models.MediaTypeMovie:
		result, err = c.SearchMovie(ctx, title, year)
	case models.MediaTypeTVShow:
		result, err = c.SearchTV(ctx, title)
	default:
		return
	}
	if err != nil {
		slog.Warn("TMDB search failed",
			"id", item.ID,
			"title", title,
			"year", year,
			"media_type", item.MediaType,
			"error", err,
		)
		return
	}
	if result == nil {
		slog.Info("no TMDB match found",
			"id", item.ID,
			"title", title,
			"year", year,
			"media_type", item.MediaType,
			"file_path", item.FilePath,
		)
		return
	}

	localPoster := ""
	if td := e.thumbsDir(ctx); td != "" && result.PosterPath != "" {
		localPoster, _ = c.DownloadPoster(ctx, result.PosterPath, td)
	}

	director := ""
	var cast []models.CastMember
	localBackdrop := ""
	var localExtraPosters []string

	details, err := c.GetMovieDetails(ctx, result.ID)
	if err == nil && details != nil {
		if td := e.thumbsDir(ctx); td != "" {
			if details.PosterPath != "" {
				if lp, err := c.DownloadPoster(ctx, details.PosterPath, td); err == nil {
					localPoster = lp
				}
			}
			if details.BackdropPath != "" {
				if lb, err := c.DownloadPoster(ctx, details.BackdropPath, td); err == nil {
					localBackdrop = lb
				}
			}
			for _, epPath := range details.ExtraPosters {
				if lep, err := c.DownloadPoster(ctx, epPath, td); err == nil {
					localExtraPosters = append(localExtraPosters, lep)
				}
			}
		}
		director = details.Director
		cast = details.Cast
		if err := sqlite.UpdateMediaMetadata(
			ctx, e.db, item.ID,
			details.ID, details.Year, details.Overview, localPoster,
			director, cast, localBackdrop, localExtraPosters,
		); err != nil {
			slog.Warn("storing TMDB metadata failed", "id", item.ID, "error", err)
		}
	} else {
		if err := sqlite.UpdateMediaMetadata(
			ctx, e.db, item.ID,
			result.ID, result.Year, result.Overview, localPoster,
			"", nil, "", nil,
		); err != nil {
			slog.Warn("storing fallback TMDB metadata failed", "id", item.ID, "error", err)
		}
	}
}

// EnrichTVShow searches TMDB for a TV show and persists show-level metadata.
// It is a no-op when the show already has a TMDB ID or when no key is set.
func (e *Enricher) EnrichTVShow(ctx context.Context, show *models.TVShow) {
	c := e.client(ctx)
	if c == nil {
		slog.Info("skipping TV show enrichment: no TMDB API key configured", "id", show.ID, "name", show.Name)
		return
	}
	if show.TMDBId != nil {
		return
	}

	result, err := c.SearchTV(ctx, show.Name)
	if err != nil {
		slog.Warn("TMDB TV search failed",
			"id", show.ID,
			"show", show.Name,
			"error", err,
		)
		return
	}
	if result == nil {
		slog.Info("no TMDB match for TV show",
			"id", show.ID,
			"show", show.Name,
		)
		return
	}

	localPoster := ""
	if td := e.thumbsDir(ctx); td != "" && result.PosterPath != "" {
		localPoster, _ = c.DownloadPoster(ctx, result.PosterPath, td)
	}

	director := ""
	var cast []models.CastMember
	localBackdrop := ""
	var localExtraPosters []string

	details, err := c.GetTVDetails(ctx, result.ID)
	if err == nil && details != nil {
		if td := e.thumbsDir(ctx); td != "" {
			if details.PosterPath != "" {
				if lp, err := c.DownloadPoster(ctx, details.PosterPath, td); err == nil {
					localPoster = lp
				}
			}
			if details.BackdropPath != "" {
				if lb, err := c.DownloadPoster(ctx, details.BackdropPath, td); err == nil {
					localBackdrop = lb
				}
			}
			for _, epPath := range details.ExtraPosters {
				if lep, err := c.DownloadPoster(ctx, epPath, td); err == nil {
					localExtraPosters = append(localExtraPosters, lep)
				}
			}
		}
		director = details.Director
		cast = details.Cast
		if err := sqlite.UpdateTVShowMetadata(
			ctx, e.db, show.ID,
			details.ID, details.Year, details.Overview, localPoster,
			director, cast, localBackdrop, localExtraPosters,
		); err != nil {
			slog.Warn("storing TV show metadata failed", "id", show.ID, "error", err)
			return
		}
	} else {
		if err := sqlite.UpdateTVShowMetadata(
			ctx, e.db, show.ID,
			result.ID, result.Year, result.Overview, localPoster,
			"", nil, "", nil,
		); err != nil {
			slog.Warn("storing TV show metadata failed", "id", show.ID, "error", err)
			return
		}
	}

	// Refresh show so callers can read the newly set tmdb_id.
	updated, err := sqlite.GetTVShowByID(ctx, e.db, show.ID)
	if err == nil {
		*show = *updated
	}
}

// EnrichTVSeason fetches season-level metadata from TMDB and persists it.
// showTMDBID must be the show's TMDB ID (obtained after EnrichTVShow).
func (e *Enricher) EnrichTVSeason(ctx context.Context, season *models.TVSeason, showTMDBID int) {
	c := e.client(ctx)
	if c == nil || season.TMDBId != nil {
		return
	}

	result, err := c.GetTVSeason(ctx, showTMDBID, season.SeasonNumber)
	if err != nil {
		slog.Warn("TMDB season fetch failed", "show_tmdb_id", showTMDBID, "season", season.SeasonNumber, "error", err)
		return
	}
	if result == nil {
		return
	}

	localPoster := ""
	if td := e.thumbsDir(ctx); td != "" && result.PosterPath != "" {
		localPoster, err = c.DownloadPoster(ctx, result.PosterPath, td)
		if err != nil {
			slog.Warn("season poster download failed", "season_id", season.ID, "error", err)
		}
	}

	if err := sqlite.UpdateTVSeasonMetadata(ctx, e.db, season.ID, result.ID, result.Overview, localPoster); err != nil {
		slog.Warn("storing TV season metadata failed", "id", season.ID, "error", err)
	}
}

// EnrichTVEpisode enriches the TV show, season, and episode in sequence.
// showID and seasonID are the DB IDs of the parent records.
func (e *Enricher) EnrichTVEpisode(ctx context.Context, item *models.MediaItem, showID, seasonID uuid.UUID) {
	c := e.client(ctx)
	if c == nil {
		slog.Info("skipping episode enrichment: no TMDB API key configured",
			"id", item.ID,
			"title", item.Title,
		)
		return
	}

	// Load show and season from DB.
	show, err := sqlite.GetTVShowByID(ctx, e.db, showID)
	if err != nil {
		slog.Warn("enrichTVEpisode: could not load show",
			"item_id", item.ID,
			"show_id", showID,
			"error", err,
		)
		return
	}
	season, err := sqlite.GetTVSeasonByID(ctx, e.db, seasonID)
	if err != nil {
		slog.Warn("enrichTVEpisode: could not load season",
			"item_id", item.ID,
			"season_id", seasonID,
			"error", err,
		)
		return
	}

	// Enrich show first to get TMDB ID.
	e.EnrichTVShow(ctx, show)
	if show.TMDBId == nil {
		slog.Info("skipping episode enrichment: show could not be matched to TMDB",
			"item_id", item.ID,
			"show", show.Name,
			"title", item.Title,
		)
		return
	}

	// Enrich season.
	e.EnrichTVSeason(ctx, season, *show.TMDBId)

	// Enrich episode.
	if item.TMDBId != nil || item.SeasonNumber == nil || item.EpisodeNumber == nil {
		return
	}

	epResult, err := c.GetTVEpisode(ctx, *show.TMDBId, *item.SeasonNumber, *item.EpisodeNumber)
	if err != nil {
		slog.Warn("TMDB episode fetch failed",
			"item_id", item.ID,
			"show", show.Name,
			"show_tmdb_id", *show.TMDBId,
			"season", *item.SeasonNumber,
			"episode", *item.EpisodeNumber,
			"error", err,
		)
		return
	}
	if epResult == nil {
		slog.Info("no TMDB match for TV episode",
			"item_id", item.ID,
			"show", show.Name,
			"show_tmdb_id", *show.TMDBId,
			"season", *item.SeasonNumber,
			"episode", *item.EpisodeNumber,
		)
		return
	}

	localStill := ""
	if td := e.thumbsDir(ctx); td != "" && epResult.StillPath != "" {
		localStill, err = c.DownloadPoster(ctx, epResult.StillPath, td)
		if err != nil {
			slog.Warn("episode still download failed", "item_id", item.ID, "error", err)
		}
	}

	if err := sqlite.UpdateMediaMetadata(ctx, e.db, item.ID, epResult.ID, epResult.AirYear, epResult.Overview, localStill, "", nil, "", nil); err != nil {
		slog.Warn("storing episode metadata failed", "id", item.ID, "error", err)
	}
}
