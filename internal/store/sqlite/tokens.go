package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ringmaster217/prism/internal/models"
)

const refreshTokenTTL = 30 * 24 * time.Hour

// CreateRefreshToken stores a new hashed refresh token for the given user.
func CreateRefreshToken(ctx context.Context, db *sql.DB, userID uuid.UUID, tokenHash string) (*models.RefreshToken, error) {
	t := &models.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().UTC().Add(refreshTokenTTL),
		CreatedAt: time.Now().UTC(),
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, revoked, created_at)
		VALUES (?, ?, ?, ?, 0, ?)`,
		t.ID.String(), t.UserID.String(), t.TokenHash,
		t.ExpiresAt.Format(time.RFC3339),
		t.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting refresh token: %w", err)
	}
	return t, nil
}

// GetRefreshTokenByHash looks up a token record by its SHA-256 hash.
func GetRefreshTokenByHash(ctx context.Context, db *sql.DB, hash string) (*models.RefreshToken, error) {
	var t models.RefreshToken
	var revoked int
	var id, userID, expiresAt, createdAt string

	err := db.QueryRowContext(ctx, `
		SELECT id, user_id, token_hash, expires_at, revoked, created_at
		FROM refresh_tokens WHERE token_hash = ?`, hash).
		Scan(&id, &userID, &t.TokenHash, &expiresAt, &revoked, &createdAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning refresh token: %w", err)
	}

	t.ID, _ = uuid.Parse(id)
	t.UserID, _ = uuid.Parse(userID)
	t.Revoked = revoked != 0
	t.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &t, nil
}

// RevokeRefreshToken marks a token as revoked by its primary key.
func RevokeRefreshToken(ctx context.Context, db *sql.DB, id uuid.UUID) error {
	_, err := db.ExecContext(ctx, `UPDATE refresh_tokens SET revoked = 1 WHERE id = ?`, id.String())
	return err
}

// DeleteExpiredTokens removes all expired refresh token rows. Call this on
// server startup and periodically to keep the table small.
func DeleteExpiredTokens(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx,
		`DELETE FROM refresh_tokens WHERE expires_at < ?`,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}
