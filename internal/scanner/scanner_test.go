package scanner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/ringmaster217/galactic-media-server/internal/models"
	"github.com/ringmaster217/galactic-media-server/internal/scanner"
	"github.com/ringmaster217/galactic-media-server/internal/store/sqlite"
	"github.com/ringmaster217/galactic-media-server/migrations"

	gosql "database/sql"
)

// openTestDB opens an in-memory SQLite DB with migrations applied.
func openTestDB(t *testing.T) *gosql.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("goose.Up: %v", err)
	}
	return db
}

// tmpLibDir creates a temporary directory with video files and returns the
// path and a cleanup function.
func tmpLibDir(t *testing.T, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		path := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fake video"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func newLibrary(t *testing.T, db *gosql.DB, dir string) *models.Library {
	t.Helper()
	lib := &models.Library{
		Name:      "Test",
		Path:      dir,
		MediaType: models.MediaTypeMovie,
	}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	return lib
}

func TestScanner_ScanAll_DiscoverFiles(t *testing.T) {
	db := openTestDB(t)
	dir := tmpLibDir(t, "movie1.mkv", "movie2.mp4", "readme.txt")
	lib := newLibrary(t, db, dir)

	s := scanner.New(db, lib, "", nil) // empty ffprobePath = skip ffprobe
	if err := s.ScanAll(context.Background()); err != nil {
		t.Fatalf("ScanAll: %v", err)
	}

	items, err := sqlite.ListMediaItems(context.Background(), db, lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("want 2 video items, got %d", len(items))
	}
}

func TestScanner_ScanAll_IgnoresNonVideo(t *testing.T) {
	db := openTestDB(t)
	dir := tmpLibDir(t, "notes.txt", "image.jpg", "subtitle.srt")
	lib := newLibrary(t, db, dir)

	s := scanner.New(db, lib, "", nil)
	if err := s.ScanAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	items, _ := sqlite.ListMediaItems(context.Background(), db, lib.ID)
	if len(items) != 0 {
		t.Errorf("want 0 items for non-video files, got %d", len(items))
	}
}

func TestScanner_ScanAll_PrunesDeletedFiles(t *testing.T) {
	db := openTestDB(t)
	dir := tmpLibDir(t, "keep.mkv", "gone.mkv")
	lib := newLibrary(t, db, dir)

	s := scanner.New(db, lib, "", nil)
	if err := s.ScanAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Delete one file from the filesystem.
	if err := os.Remove(filepath.Join(dir, "gone.mkv")); err != nil {
		t.Fatal(err)
	}

	// Scan again — should prune the deleted item.
	if err := s.ScanAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	items, _ := sqlite.ListMediaItems(context.Background(), db, lib.ID)
	if len(items) != 1 {
		t.Errorf("want 1 item after pruning, got %d", len(items))
	}
	if items[0].FilePath != filepath.Join(dir, "keep.mkv") {
		t.Errorf("remaining item = %q", items[0].FilePath)
	}
}

func TestScanner_ScanAll_Idempotent(t *testing.T) {
	db := openTestDB(t)
	dir := tmpLibDir(t, "movie.mkv")
	lib := newLibrary(t, db, dir)

	s := scanner.New(db, lib, "", nil)
	for i := 0; i < 3; i++ {
		if err := s.ScanAll(context.Background()); err != nil {
			t.Fatalf("scan %d: %v", i, err)
		}
	}

	items, _ := sqlite.ListMediaItems(context.Background(), db, lib.ID)
	if len(items) != 1 {
		t.Errorf("want exactly 1 item after 3 scans, got %d", len(items))
	}
}

func TestScanner_ScanAll_SubdirectoryRecurse(t *testing.T) {
	db := openTestDB(t)
	dir := tmpLibDir(t, "a/movie.mkv", "b/c/deep.mp4")
	lib := newLibrary(t, db, dir)

	s := scanner.New(db, lib, "", nil)
	if err := s.ScanAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	items, _ := sqlite.ListMediaItems(context.Background(), db, lib.ID)
	if len(items) != 2 {
		t.Errorf("want 2 items from subdirectories, got %d", len(items))
	}
}

func TestScanner_Watch_PicksUpNewFile(t *testing.T) {
	db := openTestDB(t)
	dir := tmpLibDir(t)
	lib := newLibrary(t, db, dir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := scanner.New(db, lib, "", nil)
	if err := s.ScanAll(ctx); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- s.Start(ctx) }()

	// Give the watcher time to initialise.
	time.Sleep(100 * time.Millisecond)

	// Create a new video file.
	newFile := filepath.Join(dir, "new.mkv")
	if err := os.WriteFile(newFile, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for the event to be processed (with a deadline).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		items, _ := sqlite.ListMediaItems(context.Background(), db, lib.ID)
		if len(items) == 1 {
			cancel()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	t.Error("timed out waiting for new file to be detected by watcher")
}
