package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ringmaster217/prism/internal/models"
	"github.com/ringmaster217/prism/internal/store/sqlite"
)

func mustNewUUID(t *testing.T) uuid.UUID {
	t.Helper()
	return uuid.New()
}

func TestCreateRefreshToken_Stored(t *testing.T) {
	sqlDB := openTestDB(t)
	u := &models.User{Username: "u1", Email: "u1@x.com", PasswordHash: "h"}
	if err := sqlite.CreateUser(context.Background(), sqlDB, u); err != nil {
		t.Fatal(err)
	}

	rt, err := sqlite.CreateRefreshToken(context.Background(), sqlDB, u.ID, "sha256hashvalue")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
	if rt.TokenHash != "sha256hashvalue" {
		t.Errorf("TokenHash = %q, want %q", rt.TokenHash, "sha256hashvalue")
	}
	if rt.ExpiresAt.IsZero() {
		t.Error("expected ExpiresAt to be set")
	}
	if rt.Revoked {
		t.Error("new token should not be revoked")
	}
}

func TestGetRefreshTokenByHash_Found(t *testing.T) {
	sqlDB := openTestDB(t)
	u := &models.User{Username: "u2", Email: "u2@x.com", PasswordHash: "h"}
	if err := sqlite.CreateUser(context.Background(), sqlDB, u); err != nil {
		t.Fatal(err)
	}

	_, err := sqlite.CreateRefreshToken(context.Background(), sqlDB, u.ID, "myhash")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := sqlite.GetRefreshTokenByHash(context.Background(), sqlDB, "myhash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.UserID != u.ID {
		t.Errorf("UserID = %v, want %v", got.UserID, u.ID)
	}
	if got.TokenHash != "myhash" {
		t.Errorf("TokenHash = %q, want %q", got.TokenHash, "myhash")
	}
}

func TestGetRefreshTokenByHash_NotFound(t *testing.T) {
	sqlDB := openTestDB(t)
	_, err := sqlite.GetRefreshTokenByHash(context.Background(), sqlDB, "nosuchhash")
	if err == nil {
		t.Fatal("expected error for missing hash, got nil")
	}
}

func TestRevokeRefreshToken(t *testing.T) {
	sqlDB := openTestDB(t)
	u := &models.User{Username: "u3", Email: "u3@x.com", PasswordHash: "h"}
	if err := sqlite.CreateUser(context.Background(), sqlDB, u); err != nil {
		t.Fatal(err)
	}

	rt, err := sqlite.CreateRefreshToken(context.Background(), sqlDB, u.ID, "revokeHash")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := sqlite.RevokeRefreshToken(context.Background(), sqlDB, rt.ID); err != nil {
		t.Fatalf("revoke error: %v", err)
	}

	got, err := sqlite.GetRefreshTokenByHash(context.Background(), sqlDB, "revokeHash")
	if err != nil {
		t.Fatalf("fetch after revoke: %v", err)
	}
	if !got.Revoked {
		t.Error("expected Revoked = true after revocation")
	}
}

func TestDeleteExpiredTokens_RemovesOnlyExpired(t *testing.T) {
	sqlDB := openTestDB(t)
	u := &models.User{Username: "u4", Email: "u4@x.com", PasswordHash: "h"}
	if err := sqlite.CreateUser(context.Background(), sqlDB, u); err != nil {
		t.Fatal(err)
	}

	// Insert a valid token via normal path.
	_, err := sqlite.CreateRefreshToken(context.Background(), sqlDB, u.ID, "validhash")
	if err != nil {
		t.Fatalf("setup valid token: %v", err)
	}

	// Insert an already-expired token directly via SQL.
	past := time.Now().UTC().Add(-1 * time.Hour)
	_, err = sqlDB.ExecContext(context.Background(), `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, revoked, created_at)
		VALUES (?, ?, ?, ?, 0, ?)`,
		uuid.New().String(), u.ID.String(), "expiredhash",
		past.Format(time.RFC3339), past.Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("setup expired token: %v", err)
	}

	if err := sqlite.DeleteExpiredTokens(context.Background(), sqlDB); err != nil {
		t.Fatalf("DeleteExpiredTokens: %v", err)
	}

	// Expired token should be gone.
	_, err = sqlite.GetRefreshTokenByHash(context.Background(), sqlDB, "expiredhash")
	if err == nil {
		t.Error("expected expired token to be deleted")
	}

	// Valid token should still exist.
	if _, err := sqlite.GetRefreshTokenByHash(context.Background(), sqlDB, "validhash"); err != nil {
		t.Errorf("valid token should still exist: %v", err)
	}
}
