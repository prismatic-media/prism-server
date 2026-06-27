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

// AddWhisperTranscription stores a Whisper transcription in the database.
func AddWhisperTranscription(ctx context.Context, db *sql.DB, t *models.WhisperTranscription) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}

	_, err := db.ExecContext(ctx, `
		INSERT OR REPLACE INTO whisper_transcriptions (id, media_item_id, language, vtt_content, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		t.ID.String(), t.MediaItemID.String(), t.Language, t.VTTContent,
		t.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("adding whisper transcription: %w", err)
	}
	return nil
}

// GetWhisperTranscriptionByMediaItem retrieves the Whisper transcription for a media item.
func GetWhisperTranscriptionByMediaItem(ctx context.Context, db *sql.DB, mediaItemID uuid.UUID) (*models.WhisperTranscription, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, media_item_id, language, vtt_content, created_at
		FROM whisper_transcriptions
		WHERE media_item_id = ?`,
		mediaItemID.String(),
	)

	var t models.WhisperTranscription
	var id, mID, createdAt string
	err := row.Scan(&id, &mID, &t.Language, &t.VTTContent, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting whisper transcription: %w", err)
	}

	t.ID, _ = uuid.Parse(id)
	t.MediaItemID, _ = uuid.Parse(mID)
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	return &t, nil
}

// HasWhisperTranscription checks whether a Whisper transcription exists for a media item.
func HasWhisperTranscription(ctx context.Context, db *sql.DB, mediaItemID uuid.UUID) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM whisper_transcriptions WHERE media_item_id = ? LIMIT 1)`,
		mediaItemID.String(),
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking whisper transcription existence: %w", err)
	}
	return exists == 1, nil
}
