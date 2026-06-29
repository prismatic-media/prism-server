package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/prismatic-media/prism-server/internal/models"
)

// ListAllMediaItemsPaged returns non-episode media items across all libraries with pagination.
func ListAllMediaItemsPaged(ctx context.Context, db *sql.DB, limit, offset int) ([]*models.MediaItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, transcode_sizes, created_at, updated_at
		FROM media_items WHERE media_type != 'episode'
		ORDER BY CASE WHEN LOWER(title) LIKE 'the %' THEN SUBSTR(title, 5) ELSE title END COLLATE NOCASE ASC, id ASC
		LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing paged media items: %w", err)
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

// ListMediaItemsPaged returns non-episode items for a given library with pagination.
func ListMediaItemsPaged(ctx context.Context, db *sql.DB, libraryID uuid.UUID, limit, offset int) ([]*models.MediaItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, transcode_sizes, created_at, updated_at
		FROM media_items WHERE library_id = ? AND media_type != 'episode'
		ORDER BY CASE WHEN LOWER(title) LIKE 'the %' THEN SUBSTR(title, 5) ELSE title END COLLATE NOCASE ASC, id ASC
		LIMIT ? OFFSET ?`, libraryID.String(), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing paged media items for library: %w", err)
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
