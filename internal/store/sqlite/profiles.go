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

// ListTranscodeProfiles returns all profiles. If onlyActive is true, it only returns active profiles.
func ListTranscodeProfiles(ctx context.Context, db *sql.DB, onlyActive bool) ([]*models.TranscodeProfile, error) {
	query := `SELECT id, name, width, height, video_bitrate_k, audio_bitrate_k, codec, is_active, created_at, updated_at
	          FROM transcode_profiles`
	if onlyActive {
		query += ` WHERE is_active = 1`
	}
	query += ` ORDER BY height ASC, video_bitrate_k ASC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing transcode profiles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var profiles []*models.TranscodeProfile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

// GetTranscodeProfile retrieves a single profile by ID.
func GetTranscodeProfile(ctx context.Context, db *sql.DB, id uuid.UUID) (*models.TranscodeProfile, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, name, width, height, video_bitrate_k, audio_bitrate_k, codec, is_active, created_at, updated_at
		FROM transcode_profiles WHERE id = ?`, id.String())
	return scanProfileRow(row)
}

// CreateTranscodeProfile inserts a new profile.
func CreateTranscodeProfile(ctx context.Context, db *sql.DB, p *models.TranscodeProfile) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	isActiveInt := 0
	if p.IsActive {
		isActiveInt = 1
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO transcode_profiles (id, name, width, height, video_bitrate_k, audio_bitrate_k, codec, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID.String(), p.Name, p.Width, p.Height, p.VideoBitrateK, p.AudioBitrateK, p.Codec, isActiveInt,
		p.CreatedAt.Format(time.RFC3339), p.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("creating transcode profile: %w", err)
	}
	return nil
}

// UpdateTranscodeProfile updates an existing profile.
func UpdateTranscodeProfile(ctx context.Context, db *sql.DB, p *models.TranscodeProfile) error {
	p.UpdatedAt = time.Now().UTC()

	isActiveInt := 0
	if p.IsActive {
		isActiveInt = 1
	}

	res, err := db.ExecContext(ctx, `
		UPDATE transcode_profiles
		SET name = ?, width = ?, height = ?, video_bitrate_k = ?, audio_bitrate_k = ?, codec = ?, is_active = ?, updated_at = ?
		WHERE id = ?`,
		p.Name, p.Width, p.Height, p.VideoBitrateK, p.AudioBitrateK, p.Codec, isActiveInt,
		p.UpdatedAt.Format(time.RFC3339), p.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("updating transcode profile: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTranscodeProfile deletes a profile by ID.
func DeleteTranscodeProfile(ctx context.Context, db *sql.DB, id uuid.UUID) error {
	res, err := db.ExecContext(ctx, `DELETE FROM transcode_profiles WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("deleting transcode profile: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanProfile(rows *sql.Rows) (*models.TranscodeProfile, error) {
	var p models.TranscodeProfile
	var id, name, codec, createdAt, updatedAt string
	var width, height, videoBitrateK, audioBitrateK, isActiveInt int

	err := rows.Scan(&id, &name, &width, &height, &videoBitrateK, &audioBitrateK, &codec, &isActiveInt, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanning transcode profile: %w", err)
	}

	p.ID, _ = uuid.Parse(id)
	p.Name = name
	p.Width = width
	p.Height = height
	p.VideoBitrateK = videoBitrateK
	p.AudioBitrateK = audioBitrateK
	p.Codec = codec
	p.IsActive = isActiveInt == 1
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &p, nil
}

func scanProfileRow(row *sql.Row) (*models.TranscodeProfile, error) {
	var p models.TranscodeProfile
	var id, name, codec, createdAt, updatedAt string
	var width, height, videoBitrateK, audioBitrateK, isActiveInt int

	err := row.Scan(&id, &name, &width, &height, &videoBitrateK, &audioBitrateK, &codec, &isActiveInt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning transcode profile: %w", err)
	}

	p.ID, _ = uuid.Parse(id)
	p.Name = name
	p.Width = width
	p.Height = height
	p.VideoBitrateK = videoBitrateK
	p.AudioBitrateK = audioBitrateK
	p.Codec = codec
	p.IsActive = isActiveInt == 1
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &p, nil
}
