package sqlite_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/ringmaster217/prism/internal/models"
	"github.com/ringmaster217/prism/internal/store/sqlite"
)

// _ suppresses unused import warning for uuid when only struct fields use it.
var _ = uuid.Nil

func TestUpsertWatchHistory_InsertAndUpdate(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Seed a user to satisfy FK.
	u := &models.User{Username: "tester", Email: "t@x.com", PasswordHash: "hash", IsAdmin: false}
	if err := sqlite.CreateUser(ctx, db, u); err != nil {
		t.Fatal(err)
	}

	// Seed prerequisite library + item.
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "Film", "/l/film.mkv")
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	userID := u.ID

	// Insert.
	h := &models.WatchHistory{
		UserID:      userID,
		MediaItemID: m.ID,
		Position:    42.5,
		Completed:   false,
	}
	if err := sqlite.UpsertWatchHistory(ctx, db, h); err != nil {
		t.Fatalf("UpsertWatchHistory insert: %v", err)
	}
	if h.ID == uuid.Nil {
		t.Error("expected ID to be set after insert")
	}

	// Update — same user+item, new position.
	h2 := &models.WatchHistory{
		UserID:      userID,
		MediaItemID: m.ID,
		Position:    120.0,
		Completed:   false,
	}
	if err := sqlite.UpsertWatchHistory(ctx, db, h2); err != nil {
		t.Fatalf("UpsertWatchHistory update: %v", err)
	}

	got, err := sqlite.GetWatchHistory(ctx, db, userID, m.ID)
	if err != nil {
		t.Fatalf("GetWatchHistory: %v", err)
	}
	if got.Position != 120.0 {
		t.Errorf("position = %.1f, want 120.0", got.Position)
	}
}

func TestGetWatchHistory_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := sqlite.GetWatchHistory(context.Background(), db, uuid.New(), uuid.New())
	if err != sqlite.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListWatchHistory_ExcludesCompleted(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Seed a user.
	u := &models.User{Username: "tester2", Email: "t2@x.com", PasswordHash: "hash", IsAdmin: false}
	if err := sqlite.CreateUser(ctx, db, u); err != nil {
		t.Fatal(err)
	}

	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m1 := newMovieItem(lib.ID, "Movie1", "/l/m1.mkv")
	m2 := newMovieItem(lib.ID, "Movie2", "/l/m2.mkv")
	if err := sqlite.UpsertMediaItem(ctx, db, m1); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpsertMediaItem(ctx, db, m2); err != nil {
		t.Fatal(err)
	}

	userID := u.ID

	// m1: in-progress.
	if err := sqlite.UpsertWatchHistory(ctx, db, &models.WatchHistory{
		UserID: userID, MediaItemID: m1.ID, Position: 30.0, Completed: false,
	}); err != nil {
		t.Fatal(err)
	}
	// m2: completed.
	if err := sqlite.UpsertWatchHistory(ctx, db, &models.WatchHistory{
		UserID: userID, MediaItemID: m2.ID, Position: 5400.0, Completed: true,
	}); err != nil {
		t.Fatal(err)
	}

	items, err := sqlite.ListWatchHistory(ctx, db, userID)
	if err != nil {
		t.Fatalf("ListWatchHistory: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	if items[0].MediaItemID != m1.ID {
		t.Errorf("unexpected item %v", items[0].MediaItemID)
	}
}

func TestListWatchHistory_Empty(t *testing.T) {
	db := openTestDB(t)
	items, err := sqlite.ListWatchHistory(context.Background(), db, uuid.New())
	if err != nil {
		t.Fatalf("ListWatchHistory: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty list, got %d items", len(items))
	}
}
