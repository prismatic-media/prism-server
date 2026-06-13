package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ringmaster217/prism/internal/models"
)

func ListStorageAreas(ctx context.Context, db *sql.DB) ([]*models.StorageArea, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, kind, path, enabled, created_at, updated_at
		FROM storage_areas
		ORDER BY kind ASC, created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing storage areas: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.StorageArea
	for rows.Next() {
		a, err := scanStorageAreaRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating storage areas: %w", err)
	}
	return out, nil
}

func ListStorageAreasByKind(ctx context.Context, db *sql.DB, kind models.StorageAreaKind, onlyEnabled bool) ([]*models.StorageArea, error) {
	query := `
		SELECT id, kind, path, enabled, created_at, updated_at
		FROM storage_areas
		WHERE kind = ?`
	args := []any{string(kind)}
	if onlyEnabled {
		query += ` AND enabled = 1`
	}
	query += ` ORDER BY created_at ASC`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing storage areas by kind: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.StorageArea
	for rows.Next() {
		a, err := scanStorageAreaRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating storage areas by kind: %w", err)
	}
	return out, nil
}

func CreateStorageArea(ctx context.Context, db *sql.DB, area *models.StorageArea) error {
	if area.ID == uuid.Nil {
		area.ID = uuid.New()
	}
	now := time.Now().UTC()
	area.CreatedAt = now
	area.UpdatedAt = now

	_, err := db.ExecContext(ctx, `
		INSERT INTO storage_areas (id, kind, path, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, area.ID.String(), string(area.Kind), area.Path, boolToInt(area.Enabled),
		now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("creating storage area: %w", err)
	}
	return nil
}

func UpdateStorageArea(ctx context.Context, db *sql.DB, id uuid.UUID, path string, enabled bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.ExecContext(ctx, `
		UPDATE storage_areas
		SET path = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, path, boolToInt(enabled), now, id.String())
	if err != nil {
		return fmt.Errorf("updating storage area: %w", err)
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

func GetStorageAreaByID(ctx context.Context, db *sql.DB, id uuid.UUID) (*models.StorageArea, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, kind, path, enabled, created_at, updated_at
		FROM storage_areas
		WHERE id = ?
	`, id.String())
	return scanStorageArea(row)
}

func BootstrapStorageAreas(ctx context.Context, db *sql.DB) error {
	return nil
}


func scanStorageArea(row *sql.Row) (*models.StorageArea, error) {
	var (
		id, kind, path, createdAt, updatedAt string
		enabled                               int
	)
	if err := row.Scan(&id, &kind, &path, &enabled, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scanning storage area: %w", err)
	}
	return populateStorageArea(id, kind, path, enabled, createdAt, updatedAt)
}

func scanStorageAreaRow(rows *sql.Rows) (*models.StorageArea, error) {
	var (
		id, kind, path, createdAt, updatedAt string
		enabled                               int
	)
	if err := rows.Scan(&id, &kind, &path, &enabled, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning storage area row: %w", err)
	}
	return populateStorageArea(id, kind, path, enabled, createdAt, updatedAt)
}

func populateStorageArea(id, kind, path string, enabled int, createdAt, updatedAt string) (*models.StorageArea, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("parsing storage area id: %w", err)
	}
	created, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parsing storage area created_at: %w", err)
	}
	updated, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parsing storage area updated_at: %w", err)
	}
	return &models.StorageArea{
		ID:        parsedID,
		Kind:      models.StorageAreaKind(kind),
		Path:      path,
		Enabled:   enabled == 1,
		CreatedAt: created,
		UpdatedAt: updated,
	}, nil
}
