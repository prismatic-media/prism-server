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

// CreateLibrary inserts a new library. l.ID is assigned by this function.
func CreateLibrary(ctx context.Context, db *sql.DB, l *models.Library) error {
	l.ID = uuid.New()
	now := time.Now().UTC()
	l.CreatedAt = now
	l.UpdatedAt = now

	_, err := db.ExecContext(ctx, `
		INSERT INTO libraries (id, name, path, media_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		l.ID.String(), l.Name, l.Path, string(l.MediaType),
		l.CreatedAt.Format(time.RFC3339),
		l.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting library: %w", err)
	}
	return nil
}

// GetLibraryByID fetches a single library by primary key.
func GetLibraryByID(ctx context.Context, db *sql.DB, id uuid.UUID) (*models.Library, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, name, path, media_type, created_at, updated_at
		FROM libraries WHERE id = ?`, id.String())
	return scanLibrary(row)
}

// ListLibraries returns all libraries ordered by name.
func ListLibraries(ctx context.Context, db *sql.DB) ([]*models.Library, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, path, media_type, created_at, updated_at
		FROM libraries ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listing libraries: %w", err)
	}
	defer rows.Close()

	var libs []*models.Library
	for rows.Next() {
		l, err := scanLibraryRow(rows)
		if err != nil {
			return nil, err
		}
		libs = append(libs, l)
	}
	return libs, rows.Err()
}

// DeleteLibrary removes a library (and its media items via CASCADE) by ID.
func DeleteLibrary(ctx context.Context, db *sql.DB, id uuid.UUID) error {
	res, err := db.ExecContext(ctx, `DELETE FROM libraries WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("deleting library: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanLibrary(row *sql.Row) (*models.Library, error) {
	var l models.Library
	var id, createdAt, updatedAt, mediaType string
	err := row.Scan(&id, &l.Name, &l.Path, &mediaType, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning library: %w", err)
	}
	l.ID, _ = uuid.Parse(id)
	l.MediaType = models.MediaType(mediaType)
	l.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	l.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &l, nil
}

func scanLibraryRow(rows *sql.Rows) (*models.Library, error) {
	var l models.Library
	var id, createdAt, updatedAt, mediaType string
	err := rows.Scan(&id, &l.Name, &l.Path, &mediaType, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanning library row: %w", err)
	}
	l.ID, _ = uuid.Parse(id)
	l.MediaType = models.MediaType(mediaType)
	l.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	l.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &l, nil
}
