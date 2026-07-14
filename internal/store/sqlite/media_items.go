package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/models"
)

// UpsertMediaItem inserts a new media item or updates the technical fields
// (file_size, duration, width, height, codecs) if a row with the same
// file_path already exists.
func UpsertMediaItem(ctx context.Context, db *sql.DB, m *models.MediaItem) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	if m.SourceStatus == "" {
		m.SourceStatus = models.SourceStatusAvailable
	}
	if m.BundleStatus == "" {
		m.BundleStatus = models.BundleStatusNone
	}
	if m.TranscodeStatus == "" {
		m.TranscodeStatus = models.TranscodeStatusNone
	}
	if m.ProbeStatus == "" {
		m.ProbeStatus = models.ProbeStatusPending
	}
	if m.EnrichmentStatus == "" {
		m.EnrichmentStatus = models.EnrichmentStatusPending
	}

	var castStr, extraPostersStr sql.NullString
	if len(m.Cast) > 0 {
		if b, err := json.Marshal(m.Cast); err == nil {
			castStr = sql.NullString{String: string(b), Valid: true}
		}
	}
	if len(m.ExtraPosters) > 0 {
		if b, err := json.Marshal(m.ExtraPosters); err == nil {
			extraPostersStr = sql.NullString{String: string(b), Valid: true}
		}
	}
	var sizesStr sql.NullString
	if m.TranscodeSizes != nil {
		if b, err := json.Marshal(m.TranscodeSizes); err == nil {
			sizesStr = sql.NullString{String: string(b), Valid: true}
		}
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO media_items
			(id, library_id, title, media_type, file_path, file_size,
			 duration, width, height, video_codec, audio_codec,
			 tv_show_id, tv_season_id, season_number, episode_number,
			 transcode_status, source_fingerprint, source_status, bundle_status,
			 probe_status, enrichment_status,
			 tmdb_id, year, overview, poster_path, mpd_path, director, cast_members, backdrop_path, extra_posters, transcode_sizes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(file_path) DO UPDATE SET
			title            = excluded.title,
			file_size        = excluded.file_size,
			duration         = excluded.duration,
			width            = excluded.width,
			height           = excluded.height,
			video_codec      = excluded.video_codec,
			audio_codec      = excluded.audio_codec,
			tv_show_id       = excluded.tv_show_id,
			tv_season_id     = excluded.tv_season_id,
			season_number    = excluded.season_number,
			episode_number   = excluded.episode_number,
			source_fingerprint = COALESCE(excluded.source_fingerprint, media_items.source_fingerprint),
			source_status    = excluded.source_status,
			bundle_status    = excluded.bundle_status,
			transcode_status = CASE WHEN media_items.transcode_status = 'done' THEN 'done' ELSE excluded.transcode_status END,
			probe_status     = excluded.probe_status,
			enrichment_status = excluded.enrichment_status,
			tmdb_id          = COALESCE(excluded.tmdb_id, media_items.tmdb_id),
			year             = COALESCE(excluded.year, media_items.year),
			overview         = COALESCE(excluded.overview, media_items.overview),
			poster_path      = COALESCE(excluded.poster_path, media_items.poster_path),
			mpd_path         = COALESCE(excluded.mpd_path, media_items.mpd_path),
			director         = COALESCE(excluded.director, media_items.director),
			cast_members     = COALESCE(excluded.cast_members, media_items.cast_members),
			backdrop_path    = COALESCE(excluded.backdrop_path, media_items.backdrop_path),
			extra_posters    = COALESCE(excluded.extra_posters, media_items.extra_posters),
			transcode_sizes  = COALESCE(excluded.transcode_sizes, media_items.transcode_sizes),
			updated_at       = excluded.updated_at`,
		m.ID.String(), m.LibraryID.String(), m.Title,
		string(m.MediaType), m.FilePath, m.FileSize,
		m.Duration, m.Width, m.Height,
		m.VideoCodec, m.AudioCodec,
		nullUUID(m.TVShowID), nullUUID(m.TVSeasonID),
		nullIntPtr(m.SeasonNumber), nullIntPtr(m.EpisodeNumber),
		string(m.TranscodeStatus),
		nullStringPtr(m.SourceFingerprint), m.SourceStatus, m.BundleStatus,
		string(m.ProbeStatus), string(m.EnrichmentStatus),
		nullIntPtr(m.TMDBId), nullIntPtr(m.Year), nullStringPtr(m.Overview), nullStringPtr(m.PosterPath), nullStringPtr(m.MPDPath),
		nullStringPtr(m.Director), castStr, nullStringPtr(m.BackdropPath), extraPostersStr,
		sizesStr,
		m.CreatedAt.Format(time.RFC3339),
		m.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("upserting media item: %w", err)
	}
	return nil
}

// GetMediaItemByID fetches a single media item by primary key.
func GetMediaItemByID(ctx context.Context, db *sql.DB, id uuid.UUID) (*models.MediaItem, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, probe_status, enrichment_status, transcode_sizes, created_at, updated_at
		FROM media_items WHERE id = ?`, id.String())
	item, err := scanMediaItem(row)
	if err != nil {
		return nil, err
	}
	prog, err := GetMediaItemTranscodeProgress(ctx, db, item.ID)
	if err == nil && prog != nil {
		item.TranscodeProgress = prog
	}
	subJobs, err := GetMediaItemLatestJobSubJobs(ctx, db, item.ID)
	if err == nil && subJobs != nil {
		item.SubJobs = subJobs
	}
	if item.MediaType == models.MediaTypeEpisode && item.TVShowID != nil {
		show, err := GetTVShowByID(ctx, db, *item.TVShowID)
		if err == nil {
			item.TVShowTitle = &show.Name
		}
	}
	return item, nil
}

// GetNextEpisode queries the next episode for a given TV show episode ID.
func GetNextEpisode(ctx context.Context, db *sql.DB, currentID uuid.UUID) (*models.MediaItem, error) {
	// 1. Get the current episode's show ID, season number, and episode number
	var showIDStr, currentIDStr string
	var seasonNum, episodeNum int
	row := db.QueryRowContext(ctx, `
		SELECT id, tv_show_id, season_number, episode_number 
		FROM media_items 
		WHERE id = ? AND media_type = 'episode'`, currentID.String())
	err := row.Scan(&currentIDStr, &showIDStr, &seasonNum, &episodeNum)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting current episode: %w", err)
	}

	// 2. Query for the next episode in the same show
	// - Either same season and higher episode number
	// - Or higher season number
	// Order by season number, then episode number, limit 1.
	nextRow := db.QueryRowContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, probe_status, enrichment_status, transcode_sizes, created_at, updated_at
		FROM media_items
		WHERE tv_show_id = ?
		  AND media_type = 'episode'
		  AND (
		    (season_number = ? AND episode_number > ?)
		    OR
		    (season_number > ?)
		  )
		ORDER BY season_number ASC, episode_number ASC
		LIMIT 1`, showIDStr, seasonNum, episodeNum, seasonNum)

	item, err := scanMediaItem(nextRow)
	if err != nil {
		return nil, err // Returns ErrNotFound if no next episode
	}

	// Populate transcode progress and subjobs if applicable
	prog, err := GetMediaItemTranscodeProgress(ctx, db, item.ID)
	if err == nil && prog != nil {
		item.TranscodeProgress = prog
	}
	subJobs, err := GetMediaItemLatestJobSubJobs(ctx, db, item.ID)
	if err == nil && subJobs != nil {
		item.SubJobs = subJobs
	}
	// Fetch and set show title
	if item.TVShowID != nil {
		show, err := GetTVShowByID(ctx, db, *item.TVShowID)
		if err == nil {
			item.TVShowTitle = &show.Name
		}
	}

	return item, nil
}

// GetMediaItemByPath looks up a media item by its file path.
func GetMediaItemByPath(ctx context.Context, db *sql.DB, path string) (*models.MediaItem, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, probe_status, enrichment_status, transcode_sizes, created_at, updated_at
		FROM media_items WHERE file_path = ?`, path)
	return scanMediaItem(row)
}

// ListMediaItems returns all non-episode items for a given library, ordered
// by title. Episodes are accessed through the TV show/season hierarchy.
func ListMediaItems(ctx context.Context, db *sql.DB, libraryID uuid.UUID) ([]*models.MediaItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, probe_status, enrichment_status, transcode_sizes, created_at, updated_at
		FROM media_items WHERE library_id = ? AND media_type != 'episode' ORDER BY CASE WHEN LOWER(title) LIKE 'the %' THEN SUBSTR(title, 5) ELSE title END COLLATE NOCASE`, libraryID.String())
	if err != nil {
		return nil, fmt.Errorf("listing media items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*models.MediaItem
	for rows.Next() {
		m, err := scanMediaItemRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}


// ListAllMediaItems returns all non-episode media items across all libraries.
func ListAllMediaItems(ctx context.Context, db *sql.DB) ([]*models.MediaItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, probe_status, enrichment_status, transcode_sizes, created_at, updated_at
		FROM media_items WHERE media_type != 'episode' ORDER BY CASE WHEN LOWER(title) LIKE 'the %' THEN SUBSTR(title, 5) ELSE title END COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("listing all media items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*models.MediaItem
	for rows.Next() {
		m, err := scanMediaItemRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

// ListRecentMediaItems returns the most recently added non-episode media items across all libraries.
func ListRecentMediaItems(ctx context.Context, db *sql.DB, limit int) ([]*models.MediaItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, probe_status, enrichment_status, transcode_sizes, created_at, updated_at
		FROM media_items WHERE media_type != 'episode' ORDER BY created_at DESC, CASE WHEN LOWER(title) LIKE 'the %' THEN SUBSTR(title, 5) ELSE title END COLLATE NOCASE ASC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("listing recent media items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*models.MediaItem
	for rows.Next() {
		m, err := scanMediaItemRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}


// DeleteMediaItem removes a media item by ID.
func DeleteMediaItem(ctx context.Context, db *sql.DB, id uuid.UUID) error {
	res, err := db.ExecContext(ctx, `DELETE FROM media_items WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("deleting media item: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetMediaItemByFingerprint looks up a media item in the given library by its source fingerprint.
func GetMediaItemByFingerprint(ctx context.Context, db *sql.DB, libraryID uuid.UUID, fingerprint string) (*models.MediaItem, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, probe_status, enrichment_status, transcode_sizes, created_at, updated_at
		FROM media_items
		WHERE library_id = ? AND source_fingerprint = ? LIMIT 1`, libraryID.String(), fingerprint)
	return scanMediaItem(row)
}

// UpdateMediaItemFilePath updates the file path of a media item and marks its source status as available.
func UpdateMediaItemFilePath(ctx context.Context, db *sql.DB, itemID uuid.UUID, newPath string) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE media_items
		SET file_path = ?, source_status = ?, updated_at = ?
		WHERE id = ?`,
		newPath, models.SourceStatusAvailable, now.Format(time.RFC3339), itemID.String())
	return err
}

// SetMediaSourceStatus updates the source status of a media item.
func SetMediaSourceStatus(ctx context.Context, db *sql.DB, itemID uuid.UUID, status string) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE media_items
		SET source_status = ?, updated_at = ?
		WHERE id = ?`,
		status, now.Format(time.RFC3339), itemID.String())
	return err
}

// SetMediaBundleStatus updates the bundle status of a media item.
func SetMediaBundleStatus(ctx context.Context, db *sql.DB, itemID uuid.UUID, status string) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE media_items
		SET bundle_status = ?, updated_at = ?
		WHERE id = ?`,
		status, now.Format(time.RFC3339), itemID.String())
	return err
}

// PruneStaleMediaItems handles library pruning. It deletes items where source is gone
// and bundle_status is 'none', and marks items with bundles as missing.
func PruneStaleMediaItems(ctx context.Context, db *sql.DB, libraryID uuid.UUID, paths []string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if len(paths) == 0 {
		// Delete items with no bundle.
		_, err := db.ExecContext(ctx, `
			DELETE FROM media_items
			WHERE library_id = ? AND bundle_status = 'none'`,
			libraryID.String())
		if err != nil {
			return err
		}
		// Mark items with bundle as missing.
		_, err = db.ExecContext(ctx, `
			UPDATE media_items
			SET source_status = ?, updated_at = ?
			WHERE library_id = ? AND bundle_status IN ('available', 'missing')`,
			models.SourceStatusMissing, now, libraryID.String())
		return err
	}

	// Delete query
	delPlaceholders := make([]byte, 0, len(paths)*2)
	delArgs := make([]any, 0, len(paths)+1)
	delArgs = append(delArgs, libraryID.String())
	for i, p := range paths {
		if i > 0 {
			delPlaceholders = append(delPlaceholders, ',')
		}
		delPlaceholders = append(delPlaceholders, '?')
		delArgs = append(delArgs, p)
	}

	delQuery := fmt.Sprintf(`
		DELETE FROM media_items
		WHERE library_id = ? AND bundle_status = 'none' AND file_path NOT IN (%s)`,
		string(delPlaceholders),
	)
	_, err := db.ExecContext(ctx, delQuery, delArgs...)
	if err != nil {
		return err
	}

	// Update query
	upPlaceholders := make([]byte, 0, len(paths)*2)
	upArgs := make([]any, 0, len(paths)+3)
	upArgs = append(upArgs, models.SourceStatusMissing, now, libraryID.String())
	for i, p := range paths {
		if i > 0 {
			upPlaceholders = append(upPlaceholders, ',')
		}
		upPlaceholders = append(upPlaceholders, '?')
		upArgs = append(upArgs, p)
	}

	upQuery := fmt.Sprintf(`
		UPDATE media_items
		SET source_status = ?, updated_at = ?
		WHERE library_id = ? AND bundle_status IN ('available', 'missing') AND file_path NOT IN (%s)`,
		string(upPlaceholders),
	)
	_, err = db.ExecContext(ctx, upQuery, upArgs...)
	return err
}

// UpdateMediaMetadata writes TMDB-sourced fields back to the media_items row.
// Passing 0 for tmdbID or year stores NULL in those columns.
// Passing "" for overview or posterPath stores NULL.
func UpdateMediaMetadata(ctx context.Context, db *sql.DB, id uuid.UUID, title string, tmdbID, year int, overview, posterPath, director string, cast []models.CastMember, backdropPath string, extraPosters []string) error {
	now := time.Now().UTC()
	var castStr, extraPostersStr sql.NullString
	if len(cast) > 0 {
		if b, err := json.Marshal(cast); err == nil {
			castStr = sql.NullString{String: string(b), Valid: true}
		}
	}
	if len(extraPosters) > 0 {
		if b, err := json.Marshal(extraPosters); err == nil {
			extraPostersStr = sql.NullString{String: string(b), Valid: true}
		}
	}

	_, err := db.ExecContext(ctx, `
		UPDATE media_items
		SET title         = ?,
		    tmdb_id       = ?,
		    year          = ?,
		    overview      = ?,
		    poster_path   = ?,
		    director      = ?,
		    cast_members  = ?,
		    backdrop_path = ?,
		    extra_posters = ?,
		    updated_at    = ?
		WHERE id = ?`,
		title,
		nullInt(tmdbID), nullInt(year),
		nullStr(overview), nullStr(posterPath),
		nullStr(director), castStr,
		nullStr(backdropPath), extraPostersStr,
		now.Format(time.RFC3339), id.String(),
	)
	if err != nil {
		return fmt.Errorf("updating media metadata: %w", err)
	}
	return nil
}

// UpdateMediaTitle updates only the title of a media item.
func UpdateMediaTitle(ctx context.Context, db *sql.DB, id uuid.UUID, title string) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE media_items
		SET title = ?, updated_at = ?
		WHERE id = ?`,
		title, now.Format(time.RFC3339), id.String(),
	)
	if err != nil {
		return fmt.Errorf("updating media title: %w", err)
	}
	return nil
}

// ClearAllMediaMetadata sets tmdb_id, year, overview, and poster_path to NULL
// for every media item, forcing the enricher to re-fetch them on the next pass.
func ClearAllMediaMetadata(ctx context.Context, db *sql.DB) (int64, error) {
	now := time.Now().UTC()
	res, err := db.ExecContext(ctx, `
		UPDATE media_items
		SET tmdb_id       = NULL,
		    year          = NULL,
		    overview      = NULL,
		    poster_path   = NULL,
		    director      = NULL,
		    cast_members  = NULL,
		    backdrop_path = NULL,
		    extra_posters = NULL,
		    updated_at    = ?`,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("clearing all media metadata: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ListAllMediaItemsAll returns every media item across all libraries,
// including TV episodes (used by the metadata refresh job).
func ListAllMediaItemsAll(ctx context.Context, db *sql.DB) ([]*models.MediaItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, probe_status, enrichment_status, transcode_sizes, created_at, updated_at
		FROM media_items ORDER BY CASE WHEN LOWER(title) LIKE 'the %' THEN SUBSTR(title, 5) ELSE title END COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("listing all media items (all types): %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*models.MediaItem
	for rows.Next() {
		m, err := scanMediaItemRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

func ListAllEpisodes(ctx context.Context, db *sql.DB) ([]*models.MediaItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, probe_status, enrichment_status, transcode_sizes, created_at, updated_at
		FROM media_items WHERE media_type = 'episode' ORDER BY CASE WHEN LOWER(title) LIKE 'the %' THEN SUBSTR(title, 5) ELSE title END COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("listing all episodes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*models.MediaItem
	for rows.Next() {
		m, err := scanMediaItemRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}


func nullInt(v int) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(v), Valid: v != 0}
}

func nullIntPtr(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

func nullStr(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}

// scanMediaItem scans a *sql.Row into a MediaItem.
func scanMediaItem(row *sql.Row) (*models.MediaItem, error) {
	var m models.MediaItem
	var id, libraryID, createdAt, updatedAt, mediaType, transcodeStatus, sourceStatus, bundleStatus, probeStatus, enrichmentStatus string
	var tmdbID, year, width, height, seasonNumber, episodeNumber sql.NullInt64
	var overview, posterPath, mpdPath, tvShowID, tvSeasonID, sourceFingerprint sql.NullString
	var duration sql.NullFloat64
	var director, castStr, backdropPath, extraPostersStr, transcodeSizesStr sql.NullString

	err := row.Scan(
		&id, &libraryID, &m.Title, &mediaType, &m.FilePath, &m.FileSize,
		&duration, &width, &height, &m.VideoCodec, &m.AudioCodec,
		&tmdbID, &year, &overview, &posterPath, &director, &castStr, &backdropPath, &extraPostersStr,
		&tvShowID, &tvSeasonID, &seasonNumber, &episodeNumber,
		&transcodeStatus, &mpdPath, &sourceFingerprint, &sourceStatus, &bundleStatus, &probeStatus, &enrichmentStatus, &transcodeSizesStr, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning media item: %w", err)
	}
	return populateMediaItem(&m, id, libraryID, mediaType, transcodeStatus,
		createdAt, updatedAt, tmdbID, year, width, height, seasonNumber, episodeNumber,
		duration, overview, posterPath, mpdPath, tvShowID, tvSeasonID,
		sourceFingerprint, sourceStatus, bundleStatus, probeStatus, enrichmentStatus, director, castStr, backdropPath, extraPostersStr, transcodeSizesStr), nil
}

// scanMediaItemRow scans a *sql.Rows into a MediaItem.
func scanMediaItemRow(rows *sql.Rows) (*models.MediaItem, error) {
	var m models.MediaItem
	var id, libraryID, createdAt, updatedAt, mediaType, transcodeStatus, sourceStatus, bundleStatus, probeStatus, enrichmentStatus string
	var tmdbID, year, width, height, seasonNumber, episodeNumber sql.NullInt64
	var overview, posterPath, mpdPath, tvShowID, tvSeasonID, sourceFingerprint sql.NullString
	var duration sql.NullFloat64
	var director, castStr, backdropPath, extraPostersStr, transcodeSizesStr sql.NullString

	err := rows.Scan(
		&id, &libraryID, &m.Title, &mediaType, &m.FilePath, &m.FileSize,
		&duration, &width, &height, &m.VideoCodec, &m.AudioCodec,
		&tmdbID, &year, &overview, &posterPath, &director, &castStr, &backdropPath, &extraPostersStr,
		&tvShowID, &tvSeasonID, &seasonNumber, &episodeNumber,
		&transcodeStatus, &mpdPath, &sourceFingerprint, &sourceStatus, &bundleStatus, &probeStatus, &enrichmentStatus, &transcodeSizesStr, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning media item row: %w", err)
	}
	return populateMediaItem(&m, id, libraryID, mediaType, transcodeStatus,
		createdAt, updatedAt, tmdbID, year, width, height, seasonNumber, episodeNumber,
		duration, overview, posterPath, mpdPath, tvShowID, tvSeasonID,
		sourceFingerprint, sourceStatus, bundleStatus, probeStatus, enrichmentStatus, director, castStr, backdropPath, extraPostersStr, transcodeSizesStr), nil
}

func populateMediaItem(
	m *models.MediaItem,
	id, libraryID, mediaType, transcodeStatus, createdAt, updatedAt string,
	tmdbID, year, width, height, seasonNumber, episodeNumber sql.NullInt64,
	duration sql.NullFloat64,
	overview, posterPath, mpdPath, tvShowID, tvSeasonID sql.NullString,
	sourceFingerprint sql.NullString, sourceStatus, bundleStatus, probeStatus, enrichmentStatus string,
	director, castStr, backdropPath, extraPostersStr, transcodeSizesStr sql.NullString,
) *models.MediaItem {
	m.ID, _ = uuid.Parse(id)
	m.LibraryID, _ = uuid.Parse(libraryID)
	m.MediaType = models.MediaType(mediaType)
	m.TranscodeStatus = models.TranscodeStatus(transcodeStatus)
	m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	m.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	if duration.Valid {
		m.Duration = duration.Float64
	}
	if width.Valid {
		m.Width = int(width.Int64)
	}
	if height.Valid {
		m.Height = int(height.Int64)
	}
	if tmdbID.Valid {
		v := int(tmdbID.Int64)
		m.TMDBId = &v
	}
	if year.Valid {
		v := int(year.Int64)
		m.Year = &v
	}
	if overview.Valid {
		m.Overview = &overview.String
	}
	if posterPath.Valid {
		m.PosterPath = &posterPath.String
	}
	if director.Valid {
		m.Director = &director.String
	}
	if castStr.Valid && castStr.String != "" {
		_ = json.Unmarshal([]byte(castStr.String), &m.Cast)
	}
	if backdropPath.Valid {
		m.BackdropPath = &backdropPath.String
	}
	if extraPostersStr.Valid && extraPostersStr.String != "" {
		_ = json.Unmarshal([]byte(extraPostersStr.String), &m.ExtraPosters)
	}
	if mpdPath.Valid {
		m.MPDPath = &mpdPath.String
	}
	if tvShowID.Valid && tvShowID.String != "" {
		id, err := uuid.Parse(tvShowID.String)
		if err == nil {
			m.TVShowID = &id
		}
	}
	if tvSeasonID.Valid && tvSeasonID.String != "" {
		id, err := uuid.Parse(tvSeasonID.String)
		if err == nil {
			m.TVSeasonID = &id
		}
	}
	if seasonNumber.Valid {
		v := int(seasonNumber.Int64)
		m.SeasonNumber = &v
	}
	if episodeNumber.Valid {
		v := int(episodeNumber.Int64)
		m.EpisodeNumber = &v
	}
	if sourceFingerprint.Valid {
		v := sourceFingerprint.String
		m.SourceFingerprint = &v
	}
	m.SourceStatus = sourceStatus
	m.BundleStatus = bundleStatus
	m.ProbeStatus = models.ProbeStatus(probeStatus)
	m.EnrichmentStatus = models.EnrichmentStatus(enrichmentStatus)
	if transcodeSizesStr.Valid && transcodeSizesStr.String != "" {
		var t models.TranscodeSizesInfo
		if err := json.Unmarshal([]byte(transcodeSizesStr.String), &t); err == nil {
			m.TranscodeSizes = &t
		}
	}
	return m
}

// GetMediaItemTranscodeProgress queries the latest transcode job's progress for a media item.
// Returns nil if no transcode job exists for this item.
func GetMediaItemTranscodeProgress(ctx context.Context, db *sql.DB, mediaItemID uuid.UUID) (*float64, error) {
	var progress float64
	err := db.QueryRowContext(ctx, `
		SELECT progress FROM transcode_jobs
		WHERE media_item_id = ?
		ORDER BY created_at DESC LIMIT 1`, mediaItemID.String()).Scan(&progress)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting transcode progress: %w", err)
	}
	return &progress, nil
}

// SearchMedia queries both media_items (movies) and tv_shows tables matching query.
func SearchMedia(ctx context.Context, db *sql.DB, query string) ([]*models.SearchResult, error) {
	likeQuery := "%" + query + "%"
	rows, err := db.QueryContext(ctx, `
		SELECT id, title, media_type, overview, poster_path, year, director, cast_members FROM (
			SELECT id, title, 'movie' AS media_type, overview, poster_path, year, director, cast_members
			FROM media_items
			WHERE media_type = 'movie' AND (
				title LIKE ? OR
				overview LIKE ? OR
				director LIKE ? OR
				cast_members LIKE ?
			)
			UNION ALL
			SELECT id, name AS title, 'tvshow' AS media_type, overview, poster_path, first_air_year AS year, director, cast_members
			FROM tv_shows
			WHERE (
				name LIKE ? OR
				overview LIKE ? OR
				director LIKE ? OR
				cast_members LIKE ?
			)
		) ORDER BY CASE WHEN LOWER(title) LIKE 'the %' THEN SUBSTR(title, 5) ELSE title END COLLATE NOCASE ASC`,
		likeQuery, likeQuery, likeQuery, likeQuery,
		likeQuery, likeQuery, likeQuery, likeQuery,
	)
	if err != nil {
		return nil, fmt.Errorf("searching media: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*models.SearchResult
	for rows.Next() {
		var r models.SearchResult
		var id, mediaType string
		var year sql.NullInt64
		var overview, posterPath, director, castStr sql.NullString

		err := rows.Scan(&id, &r.Title, &mediaType, &overview, &posterPath, &year, &director, &castStr)
		if err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}

		r.ID, _ = uuid.Parse(id)
		r.MediaType = mediaType
		if year.Valid {
			v := int(year.Int64)
			r.Year = &v
		}
		if overview.Valid {
			r.Overview = &overview.String
		}
		if posterPath.Valid {
			r.PosterPath = &posterPath.String
		}
		if director.Valid {
			r.Director = &director.String
		}
		if castStr.Valid && castStr.String != "" {
			_ = json.Unmarshal([]byte(castStr.String), &r.Cast)
		}

		results = append(results, &r)
	}

	return results, rows.Err()
}

// SearchMovies queries movies (non-episode media items) matching a search string.
func SearchMovies(ctx context.Context, db *sql.DB, query string) ([]*models.MediaItem, error) {
	likeQuery := "%" + query + "%"
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, probe_status, enrichment_status, transcode_sizes, created_at, updated_at
		FROM media_items
		WHERE media_type = 'movie' AND (
			title LIKE ? OR
			overview LIKE ? OR
			director LIKE ? OR
			cast_members LIKE ?
		) ORDER BY CASE WHEN LOWER(title) LIKE 'the %' THEN SUBSTR(title, 5) ELSE title END COLLATE NOCASE ASC`,
		likeQuery, likeQuery, likeQuery, likeQuery)
	if err != nil {
		return nil, fmt.Errorf("searching movies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*models.MediaItem
	for rows.Next() {
		m, err := scanMediaItemRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

// SetMediaTranscodeSizes updates the transcode sizes of a media item in the database.
func SetMediaTranscodeSizes(ctx context.Context, db *sql.DB, itemID uuid.UUID, sizes *models.TranscodeSizesInfo) error {
	var sizesStr sql.NullString
	if sizes != nil {
		if b, err := json.Marshal(sizes); err == nil {
			sizesStr = sql.NullString{String: string(b), Valid: true}
		}
	}
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE media_items
		SET transcode_sizes = ?, updated_at = ?
		WHERE id = ?`,
		sizesStr, now.Format(time.RFC3339), itemID.String())
	return err
}

// UpdateMediaProbeStatus updates only the probe_status field of a media item.
func UpdateMediaProbeStatus(ctx context.Context, db *sql.DB, id uuid.UUID, status models.ProbeStatus) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE media_items
		SET probe_status = ?, updated_at = ?
		WHERE id = ?`,
		string(status), now.Format(time.RFC3339), id.String())
	return err
}

// UpdateMediaEnrichmentStatus updates only the enrichment_status field of a media item.
func UpdateMediaEnrichmentStatus(ctx context.Context, db *sql.DB, id uuid.UUID, status models.EnrichmentStatus) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE media_items
		SET enrichment_status = ?, updated_at = ?
		WHERE id = ?`,
		string(status), now.Format(time.RFC3339), id.String())
	return err
}

// ListPendingProbes returns all media items that have probe_status = 'pending'.
func ListPendingProbes(ctx context.Context, db *sql.DB) ([]*models.MediaItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, probe_status, enrichment_status, transcode_sizes, created_at, updated_at
		FROM media_items WHERE probe_status = 'pending'`)
	if err != nil {
		return nil, fmt.Errorf("listing pending probes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*models.MediaItem
	for rows.Next() {
		m, err := scanMediaItemRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

// ListPendingEnrichments returns all media items that have enrichment_status = 'pending'.
func ListPendingEnrichments(ctx context.Context, db *sql.DB) ([]*models.MediaItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, probe_status, enrichment_status, transcode_sizes, created_at, updated_at
		FROM media_items WHERE enrichment_status = 'pending'`)
	if err != nil {
		return nil, fmt.Errorf("listing pending enrichments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*models.MediaItem
	for rows.Next() {
		m, err := scanMediaItemRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

// HandleMediaItemMove updates the file path, clears TMDB metadata, resets TV parent IDs,
// sets enrichment status to pending, and source status to available.
func HandleMediaItemMove(ctx context.Context, db *sql.DB, id uuid.UUID, newPath, newTitle string, newYear *int, tvShowID, tvSeasonID *uuid.UUID, seasonNum, episodeNum *int) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE media_items
		SET file_path         = ?,
		    title             = ?,
		    year              = ?,
		    tv_show_id        = ?,
		    tv_season_id      = ?,
		    season_number     = ?,
		    episode_number    = ?,
		    tmdb_id           = NULL,
		    overview          = NULL,
		    poster_path       = NULL,
		    director          = NULL,
		    cast_members      = NULL,
		    backdrop_path     = NULL,
		    extra_posters     = NULL,
		    source_status     = ?,
		    enrichment_status = ?,
		    updated_at        = ?
		WHERE id = ?`,
		newPath,
		newTitle,
		nullIntPtr(newYear),
		nullUUID(tvShowID),
		nullUUID(tvSeasonID),
		nullIntPtr(seasonNum),
		nullIntPtr(episodeNum),
		models.SourceStatusAvailable,
		models.EnrichmentStatusPending,
		now.Format(time.RFC3339),
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("handling media item move: %w", err)
	}
	return nil
}



