package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ringmaster217/galactic-media-server/internal/models"
)

// UpsertWatchHistory inserts or updates a watch-history row (keyed by user+item).
// Position is always overwritten; Completed is set to true when position
// reaches within 30 seconds of the end (callers may also set it explicitly).
func UpsertWatchHistory(ctx context.Context, db *sql.DB, h *models.WatchHistory) error {
	if h.ID == uuid.Nil {
		h.ID = uuid.New()
	}
	h.UpdatedAt = time.Now().UTC()

	_, err := db.ExecContext(ctx, `
		INSERT INTO watch_history (id, user_id, media_item_id, position, completed, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, media_item_id) DO UPDATE SET
			position   = excluded.position,
			completed  = excluded.completed,
			updated_at = excluded.updated_at`,
		h.ID.String(), h.UserID.String(), h.MediaItemID.String(),
		h.Position, boolToInt(h.Completed),
		h.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("upserting watch history: %w", err)
	}
	return nil
}

// GetWatchHistory fetches the watch-history entry for a specific user + item.
// Returns ErrNotFound if no entry exists.
func GetWatchHistory(ctx context.Context, db *sql.DB, userID, mediaItemID uuid.UUID) (*models.WatchHistory, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, user_id, media_item_id, position, completed, updated_at
		FROM watch_history WHERE user_id = ? AND media_item_id = ?`,
		userID.String(), mediaItemID.String(),
	)
	return scanHistory(row)
}

// ListWatchHistory returns all in-progress (not completed) watch-history entries
// for a user, ordered by most recently updated. Completed items are excluded so
// callers get the "Continue Watching" list.
func ListWatchHistory(ctx context.Context, db *sql.DB, userID uuid.UUID) ([]*models.WatchHistory, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, user_id, media_item_id, position, completed, updated_at
		FROM watch_history
		WHERE user_id = ? AND completed = 0
		ORDER BY updated_at DESC`,
		userID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("listing watch history: %w", err)
	}
	defer rows.Close()

	var items []*models.WatchHistory
	for rows.Next() {
		h, err := scanHistoryRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, h)
	}
	return items, rows.Err()
}

// --- internal scan helpers ---

func scanHistory(row *sql.Row) (*models.WatchHistory, error) {
	var h models.WatchHistory
	var id, userID, mediaItemID, updatedAt string
	var completed int

	if err := row.Scan(&id, &userID, &mediaItemID, &h.Position, &completed, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scanning watch history: %w", err)
	}
	return populateHistory(&h, id, userID, mediaItemID, updatedAt, completed), nil
}

func scanHistoryRow(rows *sql.Rows) (*models.WatchHistory, error) {
	var h models.WatchHistory
	var id, userID, mediaItemID, updatedAt string
	var completed int

	if err := rows.Scan(&id, &userID, &mediaItemID, &h.Position, &completed, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning watch history row: %w", err)
	}
	return populateHistory(&h, id, userID, mediaItemID, updatedAt, completed), nil
}

func populateHistory(h *models.WatchHistory, id, userID, mediaItemID, updatedAt string, completed int) *models.WatchHistory {
	h.ID, _ = uuid.Parse(id)
	h.UserID, _ = uuid.Parse(userID)
	h.MediaItemID, _ = uuid.Parse(mediaItemID)
	h.Completed = completed != 0
	h.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return h
}
