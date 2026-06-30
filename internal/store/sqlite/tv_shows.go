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

	// ON CONFLICT DO UPDATE with a trivial self-assignment keeps RETURNING
	// working on both insert and conflict paths.
	row := db.QueryRowContext(ctx, `
		INSERT INTO tv_shows (id, library_id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(library_id, name) DO UPDATE SET updated_at = tv_shows.updated_at
		RETURNING id`,
		newID.String(), show.LibraryID.String(), show.Name,
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

func UpdateTVShowMetadata(ctx context.Context, db *sql.DB, id uuid.UUID, tmdbID, firstAirYear int, overview, posterPath, director string, cast []models.CastMember, backdropPath string, extraPosters []string) error {
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
		SET tmdb_id        = ?,
		    first_air_year = ?,
		    overview       = ?,
		    poster_path    = ?,
		    director       = ?,
		    cast_members   = ?,
		    backdrop_path  = ?,
		    extra_posters  = ?,
		    updated_at     = ?
		WHERE id = ?`,
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
