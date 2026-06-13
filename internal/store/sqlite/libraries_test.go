package sqlite_test

import (
	"context"
	"testing"

	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

func newLib(path string, mt models.MediaType) *models.Library {
	return &models.Library{Path: path, MediaType: mt}
}

func TestCreateLibrary_SetsIDAndTimestamps(t *testing.T) {
	db := openTestDB(t)
	l := newLib("/media/movies", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, l); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l.ID.String() == "" {
		t.Error("expected ID to be set")
	}
	if l.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestCreateLibrary_DuplicatePathReturnsError(t *testing.T) {
	db := openTestDB(t)
	if err := sqlite.CreateLibrary(context.Background(), db, newLib("/dup", models.MediaTypeMovie)); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.CreateLibrary(context.Background(), db, newLib("/dup", models.MediaTypeMovie)); err == nil {
		t.Error("expected error for duplicate path")
	}
}

func TestGetLibraryByID_Found(t *testing.T) {
	db := openTestDB(t)
	l := newLib("/shows", models.MediaTypeTVShow)
	if err := sqlite.CreateLibrary(context.Background(), db, l); err != nil {
		t.Fatal(err)
	}
	got, err := sqlite.GetLibraryByID(context.Background(), db, l.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != l.ID {
		t.Errorf("ID = %v, want %v", got.ID, l.ID)
	}
	if got.MediaType != models.MediaTypeTVShow {
		t.Errorf("MediaType = %v, want %v", got.MediaType, models.MediaTypeTVShow)
	}
}

func TestGetLibraryByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := sqlite.GetLibraryByID(context.Background(), db, mustNewUUID(t))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListLibraries_Empty(t *testing.T) {
	db := openTestDB(t)
	libs, err := sqlite.ListLibraries(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 0 {
		t.Errorf("want 0 libraries, got %d", len(libs))
	}
}

func TestListLibraries_ReturnsAll(t *testing.T) {
	db := openTestDB(t)
	for _, name := range []string{"A", "B", "C"} {
		if err := sqlite.CreateLibrary(context.Background(), db, newLib("/"+name, models.MediaTypeMovie)); err != nil {
			t.Fatal(err)
		}
	}
	libs, err := sqlite.ListLibraries(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 3 {
		t.Errorf("want 3 libraries, got %d", len(libs))
	}
}

func TestDeleteLibrary_Removes(t *testing.T) {
	db := openTestDB(t)
	l := newLib("/del", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, l); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.DeleteLibrary(context.Background(), db, l.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := sqlite.GetLibraryByID(context.Background(), db, l.ID); err == nil {
		t.Error("expected error after deletion")
	}
}

func TestDeleteLibrary_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := sqlite.DeleteLibrary(context.Background(), db, mustNewUUID(t))
	if err == nil {
		t.Error("expected error for missing library")
	}
}
