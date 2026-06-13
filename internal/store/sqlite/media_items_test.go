package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

func newMovieItem(libraryID uuid.UUID, title, path string) *models.MediaItem {
	return &models.MediaItem{
		LibraryID:       libraryID,
		Title:           title,
		MediaType:       models.MediaTypeMovie,
		FilePath:        path,
		FileSize:        1024,
		Duration:        90.0,
		Width:           1920,
		Height:          1080,
		VideoCodec:      "h264",
		AudioCodec:      "aac",
		TranscodeStatus: models.TranscodeStatusPending,
	}
}

func TestUpsertMediaItem_Insert(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/movies", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}

	m := newMovieItem(lib.ID, "The Matrix", "/movies/matrix.mkv")
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
}

func TestUpsertMediaItem_UpdateOnConflict(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/lib", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}

	m := newMovieItem(lib.ID, "Old Title", "/lib/movie.mkv")
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}
	firstID := m.ID

	// Re-upsert with updated title — same path.
	m2 := newMovieItem(lib.ID, "New Title", "/lib/movie.mkv")
	if err := sqlite.UpsertMediaItem(context.Background(), db, m2); err != nil {
		t.Fatal(err)
	}

	got, err := sqlite.GetMediaItemByPath(context.Background(), db, "/lib/movie.mkv")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got.Title != "New Title" {
		t.Errorf("title = %q, want %q", got.Title, "New Title")
	}
	if got.ID != firstID {
		t.Error("upsert should preserve the original ID")
	}
}

func TestGetMediaItemByID_Found(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "Inception", "/l/inception.mkv")
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}
	got, err := sqlite.GetMediaItemByID(context.Background(), db, m.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != "Inception" {
		t.Errorf("title = %q", got.Title)
	}
	if got.Duration != 90.0 {
		t.Errorf("duration = %v", got.Duration)
	}
}

func TestGetMediaItemByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := sqlite.GetMediaItemByID(context.Background(), db, mustNewUUID(t))
	if err == nil {
		t.Error("expected error")
	}
}

func TestListMediaItems_FiltersByLibrary(t *testing.T) {
	db := openTestDB(t)
	lib1 := newLib("/l1", models.MediaTypeMovie)
	lib2 := newLib("/l2", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib1); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.CreateLibrary(context.Background(), db, lib2); err != nil {
		t.Fatal(err)
	}

	if err := sqlite.UpsertMediaItem(context.Background(), db, newMovieItem(lib1.ID, "A", "/l1/a.mkv")); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, newMovieItem(lib1.ID, "B", "/l1/b.mkv")); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, newMovieItem(lib2.ID, "C", "/l2/c.mkv")); err != nil {
		t.Fatal(err)
	}

	items, err := sqlite.ListMediaItems(context.Background(), db, lib1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("want 2 items for lib1, got %d", len(items))
	}

	all, err := sqlite.ListAllMediaItems(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("want 3 total items, got %d", len(all))
	}
}

func TestDeleteMediaItem(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "Delete Me", "/l/del.mkv")
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.DeleteMediaItem(context.Background(), db, m.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := sqlite.GetMediaItemByID(context.Background(), db, m.ID); err == nil {
		t.Error("expected error after deletion")
	}
}

func TestPruneStaleMediaItems_PrunesStale(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}

	// 1. Setup items:
	// a) /l/keep.mkv - bundle 'none' -> remains
	// b) /l/gone_no_bundle.mkv - bundle 'none' -> deleted
	// c) /l/gone_with_bundle.mkv - bundle 'available' -> remains with source_status missing
	mKeep := newMovieItem(lib.ID, "keep", "/l/keep.mkv")
	mGoneNoBundle := newMovieItem(lib.ID, "gone_no_bundle", "/l/gone_no_bundle.mkv")
	mGoneWithBundle := newMovieItem(lib.ID, "gone_with_bundle", "/l/gone_with_bundle.mkv")
	mGoneWithBundle.BundleStatus = models.BundleStatusAvailable

	if err := sqlite.UpsertMediaItem(context.Background(), db, mKeep); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, mGoneNoBundle); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, mGoneWithBundle); err != nil {
		t.Fatal(err)
	}

	// Run Prune
	if err := sqlite.PruneStaleMediaItems(context.Background(), db, lib.ID, []string{"/l/keep.mkv"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, err := sqlite.ListMediaItems(context.Background(), db, lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("want 2 items remaining, got %d", len(items))
	}

	// Find the items
	var gotKeep, gotGoneWithBundle *models.MediaItem
	for _, item := range items {
		switch item.FilePath {
		case "/l/keep.mkv":
			gotKeep = item
		case "/l/gone_with_bundle.mkv":
			gotGoneWithBundle = item
		}
	}

	if gotKeep == nil {
		t.Error("expected /l/keep.mkv to remain")
	} else if gotKeep.SourceStatus != models.SourceStatusAvailable {
		t.Errorf("expected keep source status available, got %s", gotKeep.SourceStatus)
	}

	if gotGoneWithBundle == nil {
		t.Error("expected /l/gone_with_bundle.mkv to remain")
	} else {
		if gotGoneWithBundle.SourceStatus != models.SourceStatusMissing {
			t.Errorf("expected gone_with_bundle source status missing, got %s", gotGoneWithBundle.SourceStatus)
		}
	}
}

func TestPruneStaleMediaItems_RemovesAllWhenEmpty(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, newMovieItem(lib.ID, "M", "/l/m.mkv")); err != nil {
		t.Fatal(err)
	}

	if err := sqlite.PruneStaleMediaItems(context.Background(), db, lib.ID, nil); err != nil {
		t.Fatal(err)
	}
	items, _ := sqlite.ListMediaItems(context.Background(), db, lib.ID)
	if len(items) != 0 {
		t.Errorf("want 0 items, got %d", len(items))
	}
}

func TestFingerprintAndStatusQueries(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}

	fp := "sha256-test-fingerprint"
	m := newMovieItem(lib.ID, "Fingerprint test", "/l/fp.mkv")
	m.SourceFingerprint = &fp
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}

	// 1. Test GetMediaItemByFingerprint
	got, err := sqlite.GetMediaItemByFingerprint(context.Background(), db, lib.ID, fp)
	if err != nil {
		t.Fatalf("GetMediaItemByFingerprint: %v", err)
	}
	if got.ID != m.ID {
		t.Errorf("GetMediaItemByFingerprint: got ID %s, want %s", got.ID, m.ID)
	}

	// 2. Test UpdateMediaItemFilePath
	newPath := "/l/new_fp.mkv"
	if err := sqlite.UpdateMediaItemFilePath(context.Background(), db, m.ID, newPath); err != nil {
		t.Fatalf("UpdateMediaItemFilePath: %v", err)
	}
	got, err = sqlite.GetMediaItemByID(context.Background(), db, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FilePath != newPath {
		t.Errorf("FilePath: got %s, want %s", got.FilePath, newPath)
	}
	if got.SourceStatus != models.SourceStatusAvailable {
		t.Errorf("SourceStatus: got %s, want %s", got.SourceStatus, models.SourceStatusAvailable)
	}

	// 3. Test SetMediaSourceStatus & SetMediaBundleStatus
	if err := sqlite.SetMediaSourceStatus(context.Background(), db, m.ID, models.SourceStatusMissing); err != nil {
		t.Fatalf("SetMediaSourceStatus: %v", err)
	}
	if err := sqlite.SetMediaBundleStatus(context.Background(), db, m.ID, models.BundleStatusAvailable); err != nil {
		t.Fatalf("SetMediaBundleStatus: %v", err)
	}

	got, err = sqlite.GetMediaItemByID(context.Background(), db, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceStatus != models.SourceStatusMissing {
		t.Errorf("SourceStatus: got %s, want %s", got.SourceStatus, models.SourceStatusMissing)
	}
	if got.BundleStatus != models.BundleStatusAvailable {
		t.Errorf("BundleStatus: got %s, want %s", got.BundleStatus, models.BundleStatusAvailable)
	}
}

// Ensure nullable fields round-trip correctly.
func TestMediaItem_NullableFields(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}

	year := 2010
	overview := "A mind-bending thriller"
	m := newMovieItem(lib.ID, "Inception", "/l/inception.mkv")
	m.Year = &year
	m.Overview = &overview
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}

	// Nullable fields are only set via direct SQL for now (metadata phase),
	// but we can verify that non-nil values survive a round-trip.
	_, err := db.ExecContext(context.Background(),
		`UPDATE media_items SET year = ?, overview = ? WHERE id = ?`,
		year, overview, m.ID.String(),
	)
	if err != nil {
		t.Fatal(err)
	}
	// Use a time placeholder to avoid flaky time comparisons.
	_ = time.Now()

	got, err := sqlite.GetMediaItemByID(context.Background(), db, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Year == nil || *got.Year != year {
		t.Errorf("Year = %v, want %d", got.Year, year)
	}
	if got.Overview == nil || *got.Overview != overview {
		t.Errorf("Overview = %v, want %q", got.Overview, overview)
	}
}

func TestUpdateMediaMetadata(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "Inception", "/l/inception.mkv")
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}

	if err := sqlite.UpdateMediaMetadata(context.Background(), db, m.ID, 27205, 2010, "A mind-bending thriller", "", "", nil, "", nil); err != nil {
		t.Fatalf("UpdateMediaMetadata: %v", err)
	}

	got, err := sqlite.GetMediaItemByID(context.Background(), db, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TMDBId == nil || *got.TMDBId != 27205 {
		t.Errorf("TMDBId = %v, want 27205", got.TMDBId)
	}
	if got.Year == nil || *got.Year != 2010 {
		t.Errorf("Year = %v, want 2010", got.Year)
	}
	if got.Overview == nil || *got.Overview != "A mind-bending thriller" {
		t.Errorf("Overview = %v", got.Overview)
	}
	// posterPath was "" so it should remain NULL.
	if got.PosterPath != nil {
		t.Errorf("PosterPath = %v, want nil (empty string should store NULL)", got.PosterPath)
	}
}

func TestUpdateMediaMetadata_ZerosTreatedAsNull(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l2", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "Test", "/l2/test.mkv")
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}

	// Passing 0 tmdbID, 0 year, "" strings — all should store as NULL.
	if err := sqlite.UpdateMediaMetadata(context.Background(), db, m.ID, 0, 0, "", "", "", nil, "", nil); err != nil {
		t.Fatalf("UpdateMediaMetadata: %v", err)
	}

	got, err := sqlite.GetMediaItemByID(context.Background(), db, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TMDBId != nil {
		t.Errorf("TMDBId = %v, want nil", got.TMDBId)
	}
	if got.Year != nil {
		t.Errorf("Year = %v, want nil", got.Year)
	}
}

func TestListMediaItems_SortingIgnoreThe(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	// Insert items in non-alphabetical order
	// Expected alphabetical sort order ignoring "The ":
	// 1. Arrival (A)
	// 2. The Matrix (Matrix - M)
	// 3. Prestige (P)
	// 4. The Terminal (Terminal - T)
	items := []*models.MediaItem{
		newMovieItem(lib.ID, "The Terminal", "/l/terminal.mkv"),
		newMovieItem(lib.ID, "Arrival", "/l/arrival.mkv"),
		newMovieItem(lib.ID, "Prestige", "/l/prestige.mkv"),
		newMovieItem(lib.ID, "The Matrix", "/l/matrix.mkv"),
	}

	for _, item := range items {
		if err := sqlite.UpsertMediaItem(ctx, db, item); err != nil {
			t.Fatal(err)
		}
	}

	got, err := sqlite.ListMediaItems(ctx, db, lib.ID)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 4 {
		t.Fatalf("expected 4 items, got %d", len(got))
	}

	expectedOrder := []string{"Arrival", "The Matrix", "Prestige", "The Terminal"}
	for i, title := range expectedOrder {
		if got[i].Title != title {
			t.Errorf("expected item %d to be %q, got %q", i, title, got[i].Title)
		}
	}
}
