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

// UpsertTVShow inserts a new TV show or, if one with the same library_id+name
// already exists, returns the existing record (show.ID is set to the canonical
// DB ID in both cases).
func UpsertTVShow(ctx context.Context, db *sql.DB, show *models.TVShow) error {
	now := time.Now().UTC()
	if show.CreatedAt.IsZero() {
		show.CreatedAt = now
	}
	show.UpdatedAt = now

	newID := uuid.New()

	row := db.QueryRowContext(ctx, `
		INSERT INTO tv_shows (id, library_id, name, first_air_year, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(library_id, name) DO UPDATE SET 
			first_air_year = COALESCE(tv_shows.first_air_year, excluded.first_air_year),
			updated_at = tv_shows.updated_at
		RETURNING id`,
		newID.String(), show.LibraryID.String(), show.Name,
		nullIntPtr(show.FirstAirYear),
		show.CreatedAt.Format(time.RFC3339), show.UpdatedAt.Format(time.RFC3339),
	)
	var id string
	if err := row.Scan(&id); err != nil {
		return fmt.Errorf("upserting tv show: %w", err)
	}
	show.ID, _ = uuid.Parse(id)
	return nil
}

// GetTVShowByID fetches a single TV show by primary key.
func GetTVShowByID(ctx context.Context, db *sql.DB, id uuid.UUID) (*models.TVShow, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, library_id, name, tmdb_id, overview, poster_path,
		       first_air_year, director, cast_members, backdrop_path, extra_posters, created_at, updated_at
		FROM tv_shows WHERE id = ?`, id.String())
	return scanTVShow(row)
}

// ListTVShows returns all TV shows for a given library, ordered by name.
func ListTVShows(ctx context.Context, db *sql.DB, libraryID uuid.UUID) ([]*models.TVShow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, name, tmdb_id, overview, poster_path,
		       first_air_year, director, cast_members, backdrop_path, extra_posters, created_at, updated_at
		FROM tv_shows WHERE library_id = ? ORDER BY CASE WHEN LOWER(name) LIKE 'the %' THEN SUBSTR(name, 5) ELSE name END COLLATE NOCASE`, libraryID.String())
	if err != nil {
		return nil, fmt.Errorf("listing tv shows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var shows []*models.TVShow
	for rows.Next() {
		s, err := scanTVShowRow(rows)
		if err != nil {
			return nil, err
		}
		shows = append(shows, s)
	}
	return shows, rows.Err()
}

func UpdateTVShowMetadata(ctx context.Context, db *sql.DB, id uuid.UUID, name string, tmdbID, firstAirYear int, overview, posterPath, director string, cast []models.CastMember, backdropPath string, extraPosters []string) error {
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
		UPDATE tv_shows
		SET name           = ?,
		    tmdb_id        = ?,
		    first_air_year = ?,
		    overview       = ?,
		    poster_path    = ?,
		    director       = ?,
		    cast_members   = ?,
		    backdrop_path  = ?,
		    extra_posters  = ?,
		    updated_at     = ?
		WHERE id = ?`,
		name,
		nullInt(tmdbID), nullInt(firstAirYear),
		nullStr(overview), nullStr(posterPath),
		nullStr(director), castStr,
		nullStr(backdropPath), extraPostersStr,
		now.Format(time.RFC3339), id.String(),
	)
	if err != nil {
		return fmt.Errorf("updating tv show metadata: %w", err)
	}
	return nil
}

// UpdateTVShowName updates only the name of a TV show.
func UpdateTVShowName(ctx context.Context, db *sql.DB, id uuid.UUID, name string) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE tv_shows
		SET name = ?, updated_at = ?
		WHERE id = ?`,
		name, now.Format(time.RFC3339), id.String(),
	)
	if err != nil {
		return fmt.Errorf("updating tv show name: %w", err)
	}
	return nil
}

// GetTVShowByTMDBId retrieves a TV show by library_id and tmdb_id.
func GetTVShowByTMDBId(ctx context.Context, db *sql.DB, libraryID uuid.UUID, tmdbID int) (*models.TVShow, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, library_id, name, tmdb_id, overview, poster_path,
		       first_air_year, director, cast_members, backdrop_path, extra_posters, created_at, updated_at
		FROM tv_shows WHERE library_id = ? AND tmdb_id = ?`, libraryID.String(), tmdbID)
	return scanTVShow(row)
}

// GetTVShowByName retrieves a TV show by library_id and name.
func GetTVShowByName(ctx context.Context, db *sql.DB, libraryID uuid.UUID, name string) (*models.TVShow, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, library_id, name, tmdb_id, overview, poster_path,
		       first_air_year, director, cast_members, backdrop_path, extra_posters, created_at, updated_at
		FROM tv_shows WHERE library_id = ? AND name = ?`, libraryID.String(), name)
	return scanTVShow(row)
}

// MergeTVShows merges one TV show into another. It moves all seasons and episodes to keepID,
// merging seasons that have conflicting season numbers, and deletes removeID.
func MergeTVShows(ctx context.Context, db *sql.DB, keepID, removeID uuid.UUID) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin merge transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Get all seasons of removeID
	rows, err := tx.QueryContext(ctx, `SELECT id, season_number FROM tv_seasons WHERE tv_show_id = ?`, removeID.String())
	if err != nil {
		return fmt.Errorf("listing remove show seasons: %w", err)
	}
	type seasonInfo struct {
		id           string
		seasonNumber int
	}
	var removeSeasons []seasonInfo
	for rows.Next() {
		var s seasonInfo
		if err := rows.Scan(&s.id, &s.seasonNumber); err != nil {
			rows.Close()
			return fmt.Errorf("scanning remove show season: %w", err)
		}
		removeSeasons = append(removeSeasons, s)
	}
	rows.Close()

	for _, rs := range removeSeasons {
		// Check if keepID already has this season number
		var keepSeasonID string
		err := tx.QueryRowContext(ctx, `SELECT id FROM tv_seasons WHERE tv_show_id = ? AND season_number = ?`, keepID.String(), rs.seasonNumber).Scan(&keepSeasonID)
		if err == nil {
			// A season with this number already exists under keepID.
			// Update all media items pointing to rs.id to point to keepSeasonID and keepID
			_, err = tx.ExecContext(ctx, `UPDATE media_items SET tv_show_id = ?, tv_season_id = ? WHERE tv_season_id = ?`, keepID.String(), keepSeasonID, rs.id)
			if err != nil {
				return fmt.Errorf("merging media items for season %d: %w", rs.seasonNumber, err)
			}
			// Delete the duplicate season record
			_, err = tx.ExecContext(ctx, `DELETE FROM tv_seasons WHERE id = ?`, rs.id)
			if err != nil {
				return fmt.Errorf("deleting duplicate season %d: %w", rs.seasonNumber, err)
			}
		} else if errors.Is(err, sql.ErrNoRows) {
			// No season with this number exists under keepID. Just update the tv_show_id of the season.
			_, err = tx.ExecContext(ctx, `UPDATE tv_seasons SET tv_show_id = ? WHERE id = ?`, keepID.String(), rs.id)
			if err != nil {
				return fmt.Errorf("reassigning season %d to keep show: %w", rs.seasonNumber, err)
			}
			// Also update media items pointing to rs.id to have the correct tv_show_id
			_, err = tx.ExecContext(ctx, `UPDATE media_items SET tv_show_id = ? WHERE tv_season_id = ?`, keepID.String(), rs.id)
			if err != nil {
				return fmt.Errorf("updating tv_show_id on media items for season %d: %w", rs.seasonNumber, err)
			}
		} else {
			return fmt.Errorf("checking season existence under keep show: %w", err)
		}
	}

	// 2. Reassign any leftover episode media items (in case of schema differences or weird records)
	_, err = tx.ExecContext(ctx, `UPDATE media_items SET tv_show_id = ? WHERE tv_show_id = ?`, keepID.String(), removeID.String())
	if err != nil {
		return fmt.Errorf("reassigning remaining media items: %w", err)
	}

	// 3. Delete the duplicate tv show
	_, err = tx.ExecContext(ctx, `DELETE FROM tv_shows WHERE id = ?`, removeID.String())
	if err != nil {
		return fmt.Errorf("deleting duplicate show: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit merge transaction: %w", err)
	}
	return nil
}


// ClearAllTVShowMetadata sets tmdb_id, first_air_year, overview, and poster_path to NULL
// for every tv_shows row, forcing the enricher to re-fetch them.
func ClearAllTVShowMetadata(ctx context.Context, db *sql.DB) error {
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE tv_shows
		SET tmdb_id        = NULL,
		    first_air_year = NULL,
		    overview       = NULL,
		    poster_path    = NULL,
		    director       = NULL,
		    cast_members   = NULL,
		    backdrop_path  = NULL,
		    extra_posters  = NULL,
		    updated_at     = ?`,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("clearing all tv show metadata: %w", err)
	}
	return nil
}

// ListAllTVShows returns all TV shows across all libraries.
func ListAllTVShows(ctx context.Context, db *sql.DB) ([]*models.TVShow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, name, tmdb_id, overview, poster_path,
		       first_air_year, director, cast_members, backdrop_path, extra_posters, created_at, updated_at
		FROM tv_shows ORDER BY CASE WHEN LOWER(name) LIKE 'the %' THEN SUBSTR(name, 5) ELSE name END COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("listing all tv shows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var shows []*models.TVShow
	for rows.Next() {
		s, err := scanTVShowRow(rows)
		if err != nil {
			return nil, err
		}
		shows = append(shows, s)
	}
	return shows, rows.Err()
}

// ListRecentTVShows returns the most recently added TV shows across all libraries.
func ListRecentTVShows(ctx context.Context, db *sql.DB, limit int) ([]*models.TVShow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, name, tmdb_id, overview, poster_path,
		       first_air_year, director, cast_members, backdrop_path, extra_posters, created_at, updated_at
		FROM tv_shows ORDER BY created_at DESC, CASE WHEN LOWER(name) LIKE 'the %' THEN SUBSTR(name, 5) ELSE name END COLLATE NOCASE ASC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("listing recent tv shows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var shows []*models.TVShow
	for rows.Next() {
		s, err := scanTVShowRow(rows)
		if err != nil {
			return nil, err
		}
		shows = append(shows, s)
	}
	return shows, rows.Err()
}

// ListAllTVShowsPaged returns TV shows across all libraries with pagination.
func ListAllTVShowsPaged(ctx context.Context, db *sql.DB, limit, offset int) ([]*models.TVShow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, name, tmdb_id, overview, poster_path,
		       first_air_year, director, cast_members, backdrop_path, extra_posters, created_at, updated_at
		FROM tv_shows ORDER BY CASE WHEN LOWER(name) LIKE 'the %' THEN SUBSTR(name, 5) ELSE name END COLLATE NOCASE
		LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing paged tv shows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var shows []*models.TVShow
	for rows.Next() {
		s, err := scanTVShowRow(rows)
		if err != nil {
			return nil, err
		}
		shows = append(shows, s)
	}
	return shows, rows.Err()
}


func scanTVShow(row *sql.Row) (*models.TVShow, error) {
	var s models.TVShow
	var id, libraryID, createdAt, updatedAt string
	var tmdbID, firstAirYear sql.NullInt64
	var overview, posterPath sql.NullString
	var director, castStr, backdropPath, extraPostersStr sql.NullString

	err := row.Scan(
		&id, &libraryID, &s.Name,
		&tmdbID, &overview, &posterPath, &firstAirYear,
		&director, &castStr, &backdropPath, &extraPostersStr,
		&createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning tv show: %w", err)
	}
	return populateTVShow(&s, id, libraryID, createdAt, updatedAt, tmdbID, firstAirYear, overview, posterPath, director, castStr, backdropPath, extraPostersStr), nil
}

func scanTVShowRow(rows *sql.Rows) (*models.TVShow, error) {
	var s models.TVShow
	var id, libraryID, createdAt, updatedAt string
	var tmdbID, firstAirYear sql.NullInt64
	var overview, posterPath sql.NullString
	var director, castStr, backdropPath, extraPostersStr sql.NullString

	err := rows.Scan(
		&id, &libraryID, &s.Name,
		&tmdbID, &overview, &posterPath, &firstAirYear,
		&director, &castStr, &backdropPath, &extraPostersStr,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning tv show row: %w", err)
	}
	return populateTVShow(&s, id, libraryID, createdAt, updatedAt, tmdbID, firstAirYear, overview, posterPath, director, castStr, backdropPath, extraPostersStr), nil
}

func populateTVShow(
	s *models.TVShow,
	id, libraryID, createdAt, updatedAt string,
	tmdbID, firstAirYear sql.NullInt64,
	overview, posterPath sql.NullString,
	director, castStr, backdropPath, extraPostersStr sql.NullString,
) *models.TVShow {
	s.ID, _ = uuid.Parse(id)
	s.LibraryID, _ = uuid.Parse(libraryID)
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if tmdbID.Valid {
		v := int(tmdbID.Int64)
		s.TMDBId = &v
	}
	if firstAirYear.Valid {
		v := int(firstAirYear.Int64)
		s.FirstAirYear = &v
	}
	if overview.Valid {
		s.Overview = &overview.String
	}
	if posterPath.Valid {
		s.PosterPath = &posterPath.String
	}
	if director.Valid {
		s.Director = &director.String
	}
	if castStr.Valid && castStr.String != "" {
		_ = json.Unmarshal([]byte(castStr.String), &s.Cast)
	}
	if backdropPath.Valid {
		s.BackdropPath = &backdropPath.String
	}
	if extraPostersStr.Valid && extraPostersStr.String != "" {
		_ = json.Unmarshal([]byte(extraPostersStr.String), &s.ExtraPosters)
	}
	return s
}

// SearchTVShows queries TV shows matching a search string.
func SearchTVShows(ctx context.Context, db *sql.DB, query string) ([]*models.TVShow, error) {
	likeQuery := "%" + query + "%"
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, name, tmdb_id, overview, poster_path, first_air_year, director, cast_members, backdrop_path, extra_posters, created_at, updated_at
		FROM tv_shows
		WHERE name LIKE ? OR
		      overview LIKE ? OR
		      director LIKE ? OR
		      cast_members LIKE ?
		ORDER BY CASE WHEN LOWER(name) LIKE 'the %' THEN SUBSTR(name, 5) ELSE name END COLLATE NOCASE ASC`,
		likeQuery, likeQuery, likeQuery, likeQuery)
	if err != nil {
		return nil, fmt.Errorf("searching tv shows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var shows []*models.TVShow
	for rows.Next() {
		s, err := scanTVShowRow(rows)
		if err != nil {
			return nil, err
		}
		shows = append(shows, s)
	}
	return shows, rows.Err()
}

// ListShowEpisodes returns all episode media items for a given TV show,
// ordered by season_number, then episode_number.
func ListShowEpisodes(ctx context.Context, db *sql.DB, showID uuid.UUID) ([]*models.MediaItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, library_id, title, media_type, file_path, file_size,
		       duration, width, height, video_codec, audio_codec,
		       tmdb_id, year, overview, poster_path, director, cast_members, backdrop_path, extra_posters,
		       tv_show_id, tv_season_id, season_number, episode_number,
		       transcode_status, mpd_path, source_fingerprint, source_status, bundle_status, probe_status, enrichment_status, transcode_sizes, created_at, updated_at
		FROM media_items
		WHERE tv_show_id = ? AND media_type = 'episode'
		ORDER BY season_number, episode_number`, showID.String())
	if err != nil {
		return nil, fmt.Errorf("listing show episodes: %w", err)
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
		subJobs, err := GetMediaItemLatestJobSubJobs(ctx, db, m.ID)
		if err == nil && subJobs != nil {
			m.SubJobs = subJobs
		}
	}
	return items, nil
}

