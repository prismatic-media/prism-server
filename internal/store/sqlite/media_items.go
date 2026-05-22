package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ringmaster217/galactic-media-server/internal/models"
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

	_, err := db.ExecContext(ctx, `
		INSERT INTO media_items
			(id, library_id, title, media_type, file_path, file_size,
			 duration, width, height, video_codec, audio_codec,
			 transcode_status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(file_path) DO UPDATE SET
			title            = excluded.title,
			file_size        = excluded.file_size,
			duration         = excluded.duration,
			width            = excluded.width,
			height           = excluded.height,
			video_codec      = excluded.video_codec,
			audio_codec      = excluded.audio_codec,
			updated_at       = excluded.updated_at`,
		m.ID.String(), m.LibraryID.String(), m.Title,
		string(m.MediaType), m.FilePath, m.FileSize,
		m.Duration, m.Width, m.Height,
		m.VideoCodec, m.AudioCodec,
		string(m.TranscodeStatus),
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
		       tmdb_id, year, overview, poster_path,
		       transcode_status, mpd_path, created_at, updated_at
		FROM media_items WHERE id = ?`, id.String())
	return scanMediaItem(row)
}

// GetMediaItemByPath looks up a media item by its file path.
func GetMediaItemByPath(ctx context.Context, db *sql.DB, path string) (*models.MediaItem, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path,
		       transcode_status, mpd_path, created_at, updated_at
		FROM media_items WHERE file_path = ?`, path)
	return scanMediaItem(row)
}

// ListMediaItems returns all items for a given library, ordered by title.
func ListMediaItems(ctx context.Context, db *sql.DB, libraryID uuid.UUID) ([]*models.MediaItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path,
		       transcode_status, mpd_path, created_at, updated_at
		FROM media_items WHERE library_id = ? ORDER BY title`, libraryID.String())
	if err != nil {
		return nil, fmt.Errorf("listing media items: %w", err)
	}
	defer rows.Close()

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

// ListAllMediaItems returns all media items across all libraries.
func ListAllMediaItems(ctx context.Context, db *sql.DB) ([]*models.MediaItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path,
		       transcode_status, mpd_path, created_at, updated_at
		FROM media_items ORDER BY title`)
	if err != nil {
		return nil, fmt.Errorf("listing all media items: %w", err)
	}
	defer rows.Close()

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

// DeleteMediaItemsNotIn removes all media items for a library whose file paths
// are not in the provided set. This is called after a scan to prune stale rows.
func DeleteMediaItemsNotIn(ctx context.Context, db *sql.DB, libraryID uuid.UUID, paths []string) error {
	if len(paths) == 0 {
		// Remove all items for this library.
		_, err := db.ExecContext(ctx,
			`DELETE FROM media_items WHERE library_id = ?`, libraryID.String())
		return err
	}

	// Build a parameterised NOT IN clause.
	args := make([]any, 0, len(paths)+1)
	args = append(args, libraryID.String())
	placeholders := make([]byte, 0, len(paths)*2)
	for i, p := range paths {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args = append(args, p)
	}

	query := fmt.Sprintf(
		`DELETE FROM media_items WHERE library_id = ? AND file_path NOT IN (%s)`,
		string(placeholders),
	)
	_, err := db.ExecContext(ctx, query, args...)
	return err
}

// UpdateMediaMetadata writes TMDB-sourced fields back to the media_items row.
// Passing 0 for tmdbID or year stores NULL in those columns.
// Passing "" for overview or posterPath stores NULL.
func UpdateMediaMetadata(ctx context.Context, db *sql.DB, id uuid.UUID, tmdbID, year int, overview, posterPath string) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE media_items
		SET tmdb_id     = ?,
		    year        = ?,
		    overview    = ?,
		    poster_path = ?,
		    updated_at  = ?
		WHERE id = ?`,
		nullInt(tmdbID), nullInt(year),
		nullStr(overview), nullStr(posterPath),
		now.Format(time.RFC3339), id.String(),
	)
	if err != nil {
		return fmt.Errorf("updating media metadata: %w", err)
	}
	return nil
}

func nullInt(v int) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(v), Valid: v != 0}
}

func nullStr(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}

// scanMediaItem scans a *sql.Row into a MediaItem.
func scanMediaItem(row *sql.Row) (*models.MediaItem, error) {
	var m models.MediaItem
	var id, libraryID, createdAt, updatedAt, mediaType, transcodeStatus string
	var tmdbID, year, width, height sql.NullInt64
	var overview, posterPath, mpdPath sql.NullString
	var duration sql.NullFloat64

	err := row.Scan(
		&id, &libraryID, &m.Title, &mediaType, &m.FilePath, &m.FileSize,
		&duration, &width, &height, &m.VideoCodec, &m.AudioCodec,
		&tmdbID, &year, &overview, &posterPath,
		&transcodeStatus, &mpdPath, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning media item: %w", err)
	}
	return populateMediaItem(&m, id, libraryID, mediaType, transcodeStatus,
		createdAt, updatedAt, tmdbID, year, width, height, duration,
		overview, posterPath, mpdPath), nil
}

// scanMediaItemRow scans a *sql.Rows into a MediaItem.
func scanMediaItemRow(rows *sql.Rows) (*models.MediaItem, error) {
	var m models.MediaItem
	var id, libraryID, createdAt, updatedAt, mediaType, transcodeStatus string
	var tmdbID, year, width, height sql.NullInt64
	var overview, posterPath, mpdPath sql.NullString
	var duration sql.NullFloat64

	err := rows.Scan(
		&id, &libraryID, &m.Title, &mediaType, &m.FilePath, &m.FileSize,
		&duration, &width, &height, &m.VideoCodec, &m.AudioCodec,
		&tmdbID, &year, &overview, &posterPath,
		&transcodeStatus, &mpdPath, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning media item row: %w", err)
	}
	return populateMediaItem(&m, id, libraryID, mediaType, transcodeStatus,
		createdAt, updatedAt, tmdbID, year, width, height, duration,
		overview, posterPath, mpdPath), nil
}

func populateMediaItem(
	m *models.MediaItem,
	id, libraryID, mediaType, transcodeStatus, createdAt, updatedAt string,
	tmdbID, year, width, height sql.NullInt64,
	duration sql.NullFloat64,
	overview, posterPath, mpdPath sql.NullString,
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
	if mpdPath.Valid {
		m.MPDPath = &mpdPath.String
	}
	return m
}
