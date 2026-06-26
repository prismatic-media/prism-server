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

// AddMediaSubtitle inserts a new subtitle. s.ID and s.CreatedAt are populated if empty/nil.
func AddMediaSubtitle(ctx context.Context, db *sql.DB, s *models.MediaSubtitle) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	if s.AlignmentStatus == "" {
		s.AlignmentStatus = "pending"
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO media_subtitles (id, media_item_id, language, label, vtt_content, similarity_score, sync_offset, alignment_status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID.String(), s.MediaItemID.String(), s.Language, s.Label, s.VTTContent,
		s.SimilarityScore, s.SyncOffset, s.AlignmentStatus,
		s.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting media subtitle: %w", err)
	}
	return nil
}

// ListMediaSubtitles retrieves all subtitles uploaded for a specific media item.
func ListMediaSubtitles(ctx context.Context, db *sql.DB, mediaItemID uuid.UUID) ([]*models.MediaSubtitle, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, media_item_id, language, label, vtt_content, similarity_score, sync_offset, alignment_status, created_at
		FROM media_subtitles
		WHERE media_item_id = ?
		ORDER BY created_at ASC`,
		mediaItemID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("querying media subtitles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var subs []*models.MediaSubtitle
	for rows.Next() {
		sub, err := scanMediaSubtitleRow(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// GetMediaSubtitleByID retrieves a single subtitle by its primary key ID.
func GetMediaSubtitleByID(ctx context.Context, db *sql.DB, id uuid.UUID) (*models.MediaSubtitle, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, media_item_id, language, label, vtt_content, similarity_score, sync_offset, alignment_status, created_at
		FROM media_subtitles
		WHERE id = ?`,
		id.String(),
	)
	return scanMediaSubtitle(row)
}

// UpdateMediaSubtitleAlignment updates the alignment status, similarity score, sync offset, and WebVTT content.
func UpdateMediaSubtitleAlignment(ctx context.Context, db *sql.DB, id uuid.UUID, status string, score *float64, offset float64, vttContent string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE media_subtitles
		SET alignment_status = ?, similarity_score = ?, sync_offset = ?, vtt_content = ?
		WHERE id = ?`,
		status, score, offset, vttContent, id.String(),
	)
	if err != nil {
		return fmt.Errorf("updating media subtitle alignment: %w", err)
	}
	return nil
}

// UpdateMediaSubtitleStatus updates only the alignment status of a subtitle.
func UpdateMediaSubtitleStatus(ctx context.Context, db *sql.DB, id uuid.UUID, status string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE media_subtitles
		SET alignment_status = ?
		WHERE id = ?`,
		status, id.String(),
	)
	if err != nil {
		return fmt.Errorf("updating media subtitle status: %w", err)
	}
	return nil
}

// DeleteMediaSubtitle deletes a subtitle by ID.
func DeleteMediaSubtitle(ctx context.Context, db *sql.DB, id uuid.UUID) error {
	res, err := db.ExecContext(ctx, `DELETE FROM media_subtitles WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("deleting media subtitle: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanMediaSubtitle(row *sql.Row) (*models.MediaSubtitle, error) {
	var s models.MediaSubtitle
	var id, mediaItemID, createdAt string
	err := row.Scan(&id, &mediaItemID, &s.Language, &s.Label, &s.VTTContent, &s.SimilarityScore, &s.SyncOffset, &s.AlignmentStatus, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning media subtitle: %w", err)
	}
	s.ID, _ = uuid.Parse(id)
	s.MediaItemID, _ = uuid.Parse(mediaItemID)
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &s, nil
}

func scanMediaSubtitleRow(rows *sql.Rows) (*models.MediaSubtitle, error) {
	var s models.MediaSubtitle
	var id, mediaItemID, createdAt string
	err := rows.Scan(&id, &mediaItemID, &s.Language, &s.Label, &s.VTTContent, &s.SimilarityScore, &s.SyncOffset, &s.AlignmentStatus, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("scanning media subtitle row: %w", err)
	}
	s.ID, _ = uuid.Parse(id)
	s.MediaItemID, _ = uuid.Parse(mediaItemID)
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &s, nil
}
