package scanner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/scanner"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/migrations"
	"github.com/prismatic-media/prism-server/pkg/events"

	gosql "database/sql"
)

// openTestDB opens an in-memory SQLite DB with migrations applied.
func openTestDB(t *testing.T) *gosql.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
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
		if err := os.WriteFile(path, []byte("fake video: "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func newLibrary(t *testing.T, db *gosql.DB, dir string) *models.Library {
	t.Helper()
	lib := &models.Library{
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

	s := scanner.New(db, lib, nil, nil) // empty ffprobePath = skip ffprobe
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

	s := scanner.New(db, lib, nil, nil)
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

	s := scanner.New(db, lib, nil, nil)
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

	s := scanner.New(db, lib, nil, nil)
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

func TestScanner_ScanAll_Optimize_EventEmission(t *testing.T) {
	db := openTestDB(t)
	dir := tmpLibDir(t, "movie.mkv")
	lib := newLibrary(t, db, dir)
	bus := events.NewBus()

	s := scanner.New(db, lib, nil, bus)

	subID, ch := bus.Subscribe()
	defer bus.Unsubscribe(subID)

	// First scan: should discover the file and emit EventMediaCreated
	if err := s.ScanAll(context.Background()); err != nil {
		t.Fatalf("first scan: %v", err)
	}

	// Read events with a short timeout
	var createdEvents []events.Event
	timeout := time.After(100 * time.Millisecond)
readLoop1:
	for {
		select {
		case evt := <-ch:
			if evt.Type == events.EventMediaCreated {
				createdEvents = append(createdEvents, evt)
			}
		case <-timeout:
			break readLoop1
		}
	}

	if len(createdEvents) != 1 {
		t.Errorf("expected 1 EventMediaCreated on first scan, got %d", len(createdEvents))
	}

	// Second scan: should skip emission since the file is unchanged
	if err := s.ScanAll(context.Background()); err != nil {
		t.Fatalf("second scan: %v", err)
	}

	// Read events with a short timeout
	var secondCreatedEvents []events.Event
	timeout2 := time.After(100 * time.Millisecond)
readLoop2:
	for {
		select {
		case evt := <-ch:
			if evt.Type == events.EventMediaCreated {
				secondCreatedEvents = append(secondCreatedEvents, evt)
			}
		case <-timeout2:
			break readLoop2
		}
	}

	if len(secondCreatedEvents) != 0 {
		t.Errorf("expected 0 EventMediaCreated on second scan, got %d", len(secondCreatedEvents))
	}
}

func TestScanner_ScanAll_RestoreMissingStatus(t *testing.T) {
	db := openTestDB(t)
	dir := tmpLibDir(t, "movie.mkv")
	lib := newLibrary(t, db, dir)
	bus := events.NewBus()

	s := scanner.New(db, lib, nil, bus)

	// First scan to create the media item.
	if err := s.ScanAll(context.Background()); err != nil {
		t.Fatalf("first scan: %v", err)
	}

	items, err := sqlite.ListMediaItems(context.Background(), db, lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item.SourceStatus != models.SourceStatusAvailable {
		t.Errorf("expected source status available, got %s", item.SourceStatus)
	}

	// Manually mark the item as missing.
	if err := sqlite.SetMediaSourceStatus(context.Background(), db, item.ID, models.SourceStatusMissing); err != nil {
		t.Fatalf("SetMediaSourceStatus: %v", err)
	}

	// Subscribe to events to verify we hit the fast path and don't re-emit EventMediaCreated
	subID, ch := bus.Subscribe()
	defer bus.Unsubscribe(subID)

	// Second scan: should restore status to available and skip heavy processing/re-emission.
	if err := s.ScanAll(context.Background()); err != nil {
		t.Fatalf("second scan: %v", err)
	}

	// Check if status is restored.
	updated, err := sqlite.GetMediaItemByID(context.Background(), db, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SourceStatus != models.SourceStatusAvailable {
		t.Errorf("expected status restored to available, got %s", updated.SourceStatus)
	}

	// Verify no EventMediaCreated event was emitted on second scan.
	timeout := time.After(100 * time.Millisecond)
	for {
		select {
		case evt := <-ch:
			if evt.Type == events.EventMediaCreated {
				t.Errorf("unexpected EventMediaCreated emitted on second scan")
			}
		case <-timeout:
			return
		}
	}
}

func TestScanner_ScanAll_SubdirectoryRecurse(t *testing.T) {
	db := openTestDB(t)
	dir := tmpLibDir(t, "a/movie.mkv", "b/c/deep.mp4")
	lib := newLibrary(t, db, dir)

	s := scanner.New(db, lib, nil, nil)
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

	s := scanner.New(db, lib, nil, nil)
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
