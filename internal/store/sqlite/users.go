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

// CountUsers returns the total number of users in the database.
func CountUsers(ctx context.Context, db *sql.DB) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser inserts a new user. u.ID is set by this function; the caller
// must have already hashed the password and stored it in u.PasswordHash.
func CreateUser(ctx context.Context, db *sql.DB, u *models.User) error {
	u.ID = uuid.New()
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now

	_, err := db.ExecContext(ctx, `
		INSERT INTO users (id, username, email, password_hash, is_admin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.ID.String(), u.Username, u.Email, u.PasswordHash,
		boolToInt(u.IsAdmin),
		u.CreatedAt.Format(time.RFC3339),
		u.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting user: %w", err)
	}
	return nil
}

// GetUserByID fetches a user by primary key.
func GetUserByID(ctx context.Context, db *sql.DB, id uuid.UUID) (*models.User, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, is_admin, created_at, updated_at
		FROM users WHERE id = ?`, id.String())
	return scanUser(row)
}

// GetUserByUsername fetches a user by username (case-sensitive).
func GetUserByUsername(ctx context.Context, db *sql.DB, username string) (*models.User, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, is_admin, created_at, updated_at
		FROM users WHERE username = ?`, username)
	return scanUser(row)
}

// UpdateUser persists changes to username, email, and password_hash.
func UpdateUser(ctx context.Context, db *sql.DB, u *models.User) error {
	u.UpdatedAt = time.Now().UTC()
	_, err := db.ExecContext(ctx, `
		UPDATE users SET username = ?, email = ?, password_hash = ?, updated_at = ?
		WHERE id = ?`,
		u.Username, u.Email, u.PasswordHash,
		u.UpdatedAt.Format(time.RFC3339),
		u.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("updating user: %w", err)
	}
	return nil
}

func scanUser(row *sql.Row) (*models.User, error) {
	var u models.User
	var isAdmin int
	var id, createdAt, updatedAt string

	err := row.Scan(&id, &u.Username, &u.Email, &u.PasswordHash, &isAdmin, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning user: %w", err)
	}

	u.ID, _ = uuid.Parse(id)
	u.IsAdmin = isAdmin != 0
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &u, nil
}
