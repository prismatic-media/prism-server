package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/models"
)

// UpsertTVSeason inserts a new season or returns the existing one. season.ID
// is set to the canonical DB ID in both cases.
func UpsertTVSeason(ctx context.Context, db *sql.DB, season *models.TVSeason) error {
	now := time.Now().UTC()
	if season.CreatedAt.IsZero() {
		season.CreatedAt = now
	}
	season.UpdatedAt = now

	newID := uuid.New()

	row := db.QueryRowContext(ctx, `
		INSERT INTO tv_seasons (id, tv_show_id, season_number, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(tv_show_id, season_number) DO UPDATE SET updated_at = tv_seasons.updated_at
		RETURNING id`,
		newID.String(), season.TVShowID.String(), season.SeasonNumber,
		season.CreatedAt.Format(time.RFC3339), season.UpdatedAt.Format(time.RFC3339),
	)
	var id string
	if err := row.Scan(&id); err != nil {
		return fmt.Errorf("upserting tv season: %w", err)
	}
	season.ID, _ = uuid.Parse(id)
	return nil
}

// GetTVSeasonByID fetches a single season by primary key.
func GetTVSeasonByID(ctx context.Context, db *sql.DB, id uuid.UUID) (*models.TVSeason, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, tv_show_id, season_number, tmdb_id, overview, poster_path, created_at, updated_at
		FROM tv_seasons WHERE id = ?`, id.String())
	return scanTVSeason(row)
}

// GetTVSeasonByShowAndNumber fetches a season by show ID + season number.
func GetTVSeasonByShowAndNumber(ctx context.Context, db *sql.DB, showID uuid.UUID, seasonNumber int) (*models.TVSeason, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, tv_show_id, season_number, tmdb_id, overview, poster_path, created_at, updated_at
		FROM tv_seasons WHERE tv_show_id = ? AND season_number = ?`,
		showID.String(), seasonNumber)
	return scanTVSeason(row)
}

// ListTVSeasons returns all seasons for a given show, ordered by season_number.
func ListTVSeasons(ctx context.Context, db *sql.DB, showID uuid.UUID) ([]*models.TVSeason, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, tv_show_id, season_number, tmdb_id, overview, poster_path, created_at, updated_at
		FROM tv_seasons WHERE tv_show_id = ? ORDER BY season_number`, showID.String())
	if err != nil {
		return nil, fmt.Errorf("listing tv seasons: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var seasons []*models.TVSeason
	for rows.Next() {
		s, err := scanTVSeasonRow(rows)
		if err != nil {
			return nil, err
		}
		seasons = append(seasons, s)
	}
	return seasons, rows.Err()
}

// ListSeasonEpisodes returns all episode media items for a given season,
// ordered by episode_number.
func ListSeasonEpisodes(ctx context.Context, db *sql.DB, seasonID uuid.UUID) ([]*models.MediaItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, transcode_sizes, created_at, updated_at
		FROM media_items
		WHERE tv_season_id = ?
		ORDER BY episode_number`, seasonID.String())
	if err != nil {
		return nil, fmt.Errorf("listing season episodes: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = rows.Close() // Close rows to release the DB connection under SetMaxOpenConns(1)

	for _, m := range items {
		prog, err := GetMediaItemTranscodeProgress(ctx, db, m.ID)
		if err == nil && prog != nil {
			m.TranscodeProgress = prog
		}
	}
	return items, nil
}

// UpdateTVSeasonMetadata writes TMDB-sourced fields to a tv_seasons row.
func UpdateTVSeasonMetadata(ctx context.Context, db *sql.DB, id uuid.UUID, tmdbID int, overview, posterPath string) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE tv_seasons
		SET tmdb_id     = ?,
		    overview    = ?,
		    poster_path = ?,
		    updated_at  = ?
		WHERE id = ?`,
		nullInt(tmdbID), nullStr(overview), nullStr(posterPath),
		now.Format(time.RFC3339), id.String(),
	)
	if err != nil {
		return fmt.Errorf("updating tv season metadata: %w", err)
	}
	return nil
}

// ClearAllTVSeasonMetadata sets tmdb_id, overview, and poster_path to NULL
// for every tv_seasons row, forcing the enricher to re-fetch them.
func ClearAllTVSeasonMetadata(ctx context.Context, db *sql.DB) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE tv_seasons
		SET tmdb_id     = NULL,
		    overview    = NULL,
		    poster_path = NULL,
		    updated_at  = ?`,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("clearing all tv season metadata: %w", err)
	}
	return nil
}

func scanTVSeason(row *sql.Row) (*models.TVSeason, error) {
	var s models.TVSeason
	var id, showID, createdAt, updatedAt string
	var tmdbID sql.NullInt64
	var overview, posterPath sql.NullString

	err := row.Scan(
		&id, &showID, &s.SeasonNumber,
		&tmdbID, &overview, &posterPath,
		&createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning tv season: %w", err)
	}
	return populateTVSeason(&s, id, showID, createdAt, updatedAt, tmdbID, overview, posterPath), nil
}

func scanTVSeasonRow(rows *sql.Rows) (*models.TVSeason, error) {
	var s models.TVSeason
	var id, showID, createdAt, updatedAt string
	var tmdbID sql.NullInt64
	var overview, posterPath sql.NullString

	err := rows.Scan(
		&id, &showID, &s.SeasonNumber,
		&tmdbID, &overview, &posterPath,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning tv season row: %w", err)
	}
	return populateTVSeason(&s, id, showID, createdAt, updatedAt, tmdbID, overview, posterPath), nil
}

func populateTVSeason(
	s *models.TVSeason,
	id, showID, createdAt, updatedAt string,
	tmdbID sql.NullInt64,
	overview, posterPath sql.NullString,
) *models.TVSeason {
	s.ID, _ = uuid.Parse(id)
	s.TVShowID, _ = uuid.Parse(showID)
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if tmdbID.Valid {
		v := int(tmdbID.Int64)
		s.TMDBId = &v
	}
	if overview.Valid {
		s.Overview = &overview.String
	}
	if posterPath.Valid {
		s.PosterPath = &posterPath.String
	}
	return s
}
