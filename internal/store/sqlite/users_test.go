package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

func newTestUser(username, email, hash string, isAdmin bool) *models.User {
	return &models.User{
		Username:     username,
		Email:        email,
		PasswordHash: hash,
		IsAdmin:      isAdmin,
	}
}

func TestCountUsers_EmptyDB(t *testing.T) {
	db := openTestDB(t)
	n, err := sqlite.CountUsers(context.Background(), db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
}

func TestCreateUser_SetsIDAndTimestamps(t *testing.T) {
	db := openTestDB(t)
	u := newTestUser("alice", "alice@example.com", "hash", false)
	if err := sqlite.CreateUser(context.Background(), db, u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.ID.String() == "" {
		t.Error("expected ID to be set")
	}
	if u.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if u.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestCreateUser_CountIncreases(t *testing.T) {
	db := openTestDB(t)
	for i, name := range []string{"a", "b", "c"} {
		u := newTestUser(name, name+"@x.com", "h", false)
		if err := sqlite.CreateUser(context.Background(), db, u); err != nil {
			t.Fatalf("CreateUser %d: %v", i, err)
		}
		n, _ := sqlite.CountUsers(context.Background(), db)
		if n != i+1 {
			t.Errorf("after %d inserts, count = %d", i+1, n)
		}
	}
}

func TestCreateUser_DuplicateUsernameReturnsError(t *testing.T) {
	db := openTestDB(t)
	u1 := newTestUser("dup", "dup1@example.com", "h", false)
	u2 := newTestUser("dup", "dup2@example.com", "h", false)
	if err := sqlite.CreateUser(context.Background(), db, u1); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}
	if err := sqlite.CreateUser(context.Background(), db, u2); err == nil {
		t.Error("expected error on duplicate username, got nil")
	}
}

func TestGetUserByID_Found(t *testing.T) {
	db := openTestDB(t)
	u := newTestUser("bob", "bob@example.com", "myhash", true)
	if err := sqlite.CreateUser(context.Background(), db, u); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := sqlite.GetUserByID(context.Background(), db, u.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("ID = %v, want %v", got.ID, u.ID)
	}
	if got.Username != "bob" {
		t.Errorf("Username = %q, want %q", got.Username, "bob")
	}
	if !got.IsAdmin {
		t.Error("expected IsAdmin = true")
	}
	if got.PasswordHash != "myhash" {
		t.Errorf("PasswordHash = %q, want %q", got.PasswordHash, "myhash")
	}
}

func TestGetUserByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	id := mustNewUUID(t)
	_, err := sqlite.GetUserByID(context.Background(), db, id)
	if err == nil {
		t.Fatal("expected error for missing user, got nil")
	}
}

func TestGetUserByUsername_Found(t *testing.T) {
	db := openTestDB(t)
	u := newTestUser("carol", "carol@example.com", "h", false)
	if err := sqlite.CreateUser(context.Background(), db, u); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := sqlite.GetUserByUsername(context.Background(), db, "carol")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Username != "carol" {
		t.Errorf("Username = %q, want %q", got.Username, "carol")
	}
}

func TestGetUserByUsername_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := sqlite.GetUserByUsername(context.Background(), db, "ghost")
	if err == nil {
		t.Fatal("expected error for missing user, got nil")
	}
}

func TestUpdateUser_ChangesFields(t *testing.T) {
	db := openTestDB(t)
	u := newTestUser("dave", "dave@example.com", "oldhash", false)
	if err := sqlite.CreateUser(context.Background(), db, u); err != nil {
		t.Fatalf("setup: %v", err)
	}

	u.Username = "david"
	u.Email = "david@example.com"
	u.PasswordHash = "newhash"
	if err := sqlite.UpdateUser(context.Background(), db, u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := sqlite.GetUserByID(context.Background(), db, u.ID)
	if err != nil {
		t.Fatalf("fetch after update: %v", err)
	}
	if got.Username != "david" {
		t.Errorf("Username = %q, want %q", got.Username, "david")
	}
	if got.Email != "david@example.com" {
		t.Errorf("Email = %q, want %q", got.Email, "david@example.com")
	}
	if got.PasswordHash != "newhash" {
		t.Errorf("PasswordHash = %q, want %q", got.PasswordHash, "newhash")
	}
	if got.UpdatedAt.Before(u.CreatedAt.Truncate(time.Second)) {
		t.Error("UpdatedAt should be >= CreatedAt (second precision) after update")
	}
}
