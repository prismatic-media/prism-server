package scanner

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/ringmaster217/prism/internal/artifact"
	"github.com/ringmaster217/prism/internal/models"
	"github.com/ringmaster217/prism/internal/store/sqlite"
	"github.com/ringmaster217/prism/internal/transcoder"
	"github.com/ringmaster217/prism/pkg/dash"
	"github.com/ringmaster217/prism/pkg/events"
	"github.com/ringmaster217/prism/pkg/fingerprint"
)

func setupTestEnv(t *testing.T) (*sql.DB, string, string) {
	t.Helper()
	dbDir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dbDir, "test.db"))
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("sqlite.Migrate: %v", err)
	}

	libraryDir := t.TempDir()
	storageDir := t.TempDir()

	return db, libraryDir, storageDir
}

func TestIntegration_DiscoveryDeduplication(t *testing.T) {
	ctx := context.Background()
	db, libraryDir, storageDir := setupTestEnv(t)

	// 1. Create a library & storage area
	lib := &models.Library{
		Path:      libraryDir,
		MediaType: models.MediaTypeMovie,
	}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	area := &models.StorageArea{
		Kind:    models.StorageAreaKindSegments,
		Path:    storageDir,
		Enabled: true,
	}
	if err := sqlite.CreateStorageArea(ctx, db, area); err != nil {
		t.Fatal(err)
	}

	// 2. Add movie file
	moviePath := filepath.Join(libraryDir, "dedup-movie.mkv")
	if err := os.WriteFile(moviePath, []byte("dedup-movie-content"), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Scan the library first time
	bus := events.NewBus()
	s := New(db, lib, nil, bus)
	if err := s.ScanAll(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify it was created
	item, err := sqlite.GetMediaItemByPath(ctx, db, moviePath)
	if err != nil {
		t.Fatal(err)
	}
	if item.SourceFingerprint == nil || *item.SourceFingerprint == "" {
		t.Error("expected fingerprint to be set")
	}

	// 4. Create fake bundle in segment area matching this fingerprint
	outputDir := filepath.Join(storageDir, item.ID.String())
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(outputDir, "manifest.mpd"), []byte("<MPD/>"), 0644)
	os.WriteFile(filepath.Join(outputDir, "media_0.mp4"), []byte("segments"), 0644)

	sc := &artifact.SidecarMetadata{
		Version:           2,
		MediaItemID:       item.ID.String(),
		SourcePath:        moviePath,
		SourceFingerprint: *item.SourceFingerprint,
		OutputDir:         outputDir,
		MPDPath:           "manifest.mpd",
	}
	if err := artifact.WriteSidecar(outputDir, sc); err != nil {
		t.Fatal(err)
	}

	// Register artifact record in DB
	indexer := NewIndexer(db, bus)
	if _, err := indexer.IndexStorageArea(ctx, area); err != nil {
		t.Fatal(err)
	}

	// 5. Remove movie file, run ScanAll. Item survives because bundle exists!
	if err := os.Remove(moviePath); err != nil {
		t.Fatal(err)
	}
	if err := s.ScanAll(ctx); err != nil {
		t.Fatal(err)
	}

	item, err = sqlite.GetMediaItemByID(ctx, db, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.SourceStatus != models.SourceStatusMissing {
		t.Errorf("expected source status missing, got %s", item.SourceStatus)
	}
	if item.BundleStatus != models.BundleStatusAvailable {
		t.Errorf("expected bundle status available, got %s", item.BundleStatus)
	}

	// 6. Put the file at a new path (rename/move)
	newMoviePath := filepath.Join(libraryDir, "moved-movie.mkv")
	if err := os.WriteFile(newMoviePath, []byte("dedup-movie-content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run ScanAll. Deduplication should detect move and update path!
	if err := s.ScanAll(ctx); err != nil {
		t.Fatal(err)
	}

	item, err = sqlite.GetMediaItemByID(ctx, db, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.FilePath != newMoviePath {
		t.Errorf("expected file path updated to %s, got %s", newMoviePath, item.FilePath)
	}
	if item.SourceStatus != models.SourceStatusAvailable {
		t.Errorf("expected source status available, got %s", item.SourceStatus)
	}
}

func TestIntegration_BundleOnlyLifecycle(t *testing.T) {
	ctx := context.Background()
	db, libraryDir, _ := setupTestEnv(t)

	lib := &models.Library{
		Path:      libraryDir,
		MediaType: models.MediaTypeMovie,
	}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	moviePath := filepath.Join(libraryDir, "bundle-movie.mkv")
	if err := os.WriteFile(moviePath, []byte("movie-data"), 0644); err != nil {
		t.Fatal(err)
	}

	bus := events.NewBus()
	s := New(db, lib, nil, bus)
	if err := s.ScanAll(ctx); err != nil {
		t.Fatal(err)
	}

	item, err := sqlite.GetMediaItemByPath(ctx, db, moviePath)
	if err != nil {
		t.Fatal(err)
	}

	// Mark bundle available in DB (simulates transcode output)
	if err := sqlite.SetMediaBundleStatus(ctx, db, item.ID, models.BundleStatusAvailable); err != nil {
		t.Fatal(err)
	}

	// Remove source file and scan
	os.Remove(moviePath)
	if err := s.ScanAll(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify item is preserved with source_status missing
	item, err = sqlite.GetMediaItemByID(ctx, db, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.SourceStatus != models.SourceStatusMissing {
		t.Errorf("expected source status missing, got %s", item.SourceStatus)
	}
}

func TestIntegration_DBRebuild(t *testing.T) {
	ctx := context.Background()
	db, libraryDir, storageDir := setupTestEnv(t)

	lib := &models.Library{
		Path:      libraryDir,
		MediaType: models.MediaTypeMovie,
	}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	area := &models.StorageArea{
		Kind:    models.StorageAreaKindSegments,
		Path:    storageDir,
		Enabled: true,
	}
	if err := sqlite.CreateStorageArea(ctx, db, area); err != nil {
		t.Fatal(err)
	}

	// 1. Create a source file in the library
	moviePath := filepath.Join(libraryDir, "rebuild-movie.mkv")
	if err := os.WriteFile(moviePath, []byte("movie-data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run scanner to create the MediaItem
	bus := events.NewBus()
	s := New(db, lib, nil, bus)
	if err := s.ScanAll(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify media item exists and calculate its fingerprint
	item, err := sqlite.GetMediaItemByPath(ctx, db, moviePath)
	if err != nil {
		t.Fatal(err)
	}
	fp, err := fingerprint.GenerateDeterministic(moviePath)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Create a bundle directory on disk with sidecar
	outputDir := filepath.Join(storageDir, item.ID.String())
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(outputDir, "manifest.mpd"), []byte("<MPD/>"), 0644)
	os.WriteFile(filepath.Join(outputDir, "media_0.mp4"), []byte("segments"), 0644)

	sc := &artifact.SidecarMetadata{
		Version:           2,
		MediaItemID:       item.ID.String(),
		SourcePath:        moviePath,
		SourceFingerprint: fp,
		OutputDir:         outputDir,
		MPDPath:           "manifest.mpd",
	}
	if err := artifact.WriteSidecar(outputDir, sc); err != nil {
		t.Fatal(err)
	}

	// 3. Run indexer. It should find the existing MediaItem and link it!
	indexer := NewIndexer(db, bus)
	sum, err := indexer.IndexStorageArea(ctx, area)
	if err != nil {
		t.Fatal(err)
	}

	if sum.Registered != 1 {
		t.Errorf("expected 1 transcode bundle registered, got %d", sum.Registered)
	}

	// 4. Verify media item is linked
	item, err = sqlite.GetMediaItemByID(ctx, db, item.ID)
	if err != nil {
		t.Fatal(err)
	}

	if item.SourceStatus != models.SourceStatusAvailable {
		t.Errorf("SourceStatus: got %s, want available", item.SourceStatus)
	}
	if item.BundleStatus != models.BundleStatusAvailable {
		t.Errorf("BundleStatus: got %s, want available", item.BundleStatus)
	}
	if item.MPDPath == nil || *item.MPDPath != filepath.Join(outputDir, "manifest.mpd") {
		t.Errorf("MPDPath: got %v, want %v", item.MPDPath, filepath.Join(outputDir, "manifest.mpd"))
	}
}

func TestIntegration_AutoEnqueueGuard(t *testing.T) {
	ctx := context.Background()
	db, libraryDir, _ := setupTestEnv(t)

	lib := &models.Library{
		Path:      libraryDir,
		MediaType: models.MediaTypeMovie,
	}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	m := &models.MediaItem{
		LibraryID:       lib.ID,
		Title:           "AutoGuard Video",
		MediaType:       models.MediaTypeMovie,
		FilePath:        filepath.Join(libraryDir, "guard.mkv"),
		FileSize:        1024,
		TranscodeStatus: models.TranscodeStatusDone,
		BundleStatus:    models.BundleStatusAvailable,
	}
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	// Create pool and try to enqueue
	cache := &dash.Cache{}
	bus := events.NewBus()
	pool := transcoder.NewPool(db, 1, cache, bus)

	_, err := pool.Enqueue(ctx, m.ID)
	if err == nil {
		t.Error("expected error when enqueuing already transcoded item with available bundle")
	}
}

func TestIntegration_SegmentsFirstThenScan_ExistingID(t *testing.T) {
	ctx := context.Background()
	db, libraryDir, storageDir := setupTestEnv(t)

	// 1. Create a library & storage area
	lib := &models.Library{
		Path:      libraryDir,
		MediaType: models.MediaTypeMovie,
	}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	area := &models.StorageArea{
		Kind:    models.StorageAreaKindSegments,
		Path:    storageDir,
		Enabled: true,
	}
	if err := sqlite.CreateStorageArea(ctx, db, area); err != nil {
		t.Fatal(err)
	}

	// 2. Pre-create the MediaItem in the DB with a specific ID, but no fingerprint
	moviePath := filepath.Join(libraryDir, "segments-first-existing.mkv")
	oldUUID := uuid.New()
	m := &models.MediaItem{
		ID:              oldUUID,
		LibraryID:       lib.ID,
		Title:           "Existing Video",
		MediaType:       models.MediaTypeMovie,
		FilePath:        moviePath,
		FileSize:        1024,
		TranscodeStatus: models.TranscodeStatusPending,
		BundleStatus:    models.BundleStatusNone,
	}
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	// 3. Prepare segments/fingerprint
	movieContent := []byte("segments-first-existing-content")
	tempMovieFile := filepath.Join(t.TempDir(), "temp.mkv")
	if err := os.WriteFile(tempMovieFile, movieContent, 0644); err != nil {
		t.Fatal(err)
	}
	fp, err := fingerprint.GenerateDeterministic(tempMovieFile)
	if err != nil {
		t.Fatal(err)
	}

	// 4. Create transcode bundle first in storage area with sidecar
	bundleID := "fake-bundle-existing-uuid-5678"
	outputDir := filepath.Join(storageDir, bundleID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(outputDir, "manifest.mpd"), []byte("<MPD/>"), 0644)
	os.WriteFile(filepath.Join(outputDir, "media_0.mp4"), []byte("segments"), 0644)

	sc := &artifact.SidecarMetadata{
		Version:           2,
		MediaItemID:       bundleID,
		SourcePath:        moviePath,
		SourceFingerprint: fp,
		OutputDir:         outputDir,
		MPDPath:           "manifest.mpd",
	}
	if err := artifact.WriteSidecar(outputDir, sc); err != nil {
		t.Fatal(err)
	}

	// Index the storage area (creates artifact records)
	bus := events.NewBus()
	indexer := NewIndexer(db, bus)
	sum, err := indexer.IndexStorageArea(ctx, area)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Registered != 1 {
		t.Errorf("expected 1 transcode bundle registered, got %d", sum.Registered)
	}

	// 5. Now write the actual movie file to the library
	if err := os.WriteFile(moviePath, movieContent, 0644); err != nil {
		t.Fatal(err)
	}

	// 6. Scan the library. The scanner should discover the movie file,
	// match it against the existing artifact record, and automatically link it.
	s := New(db, lib, nil, bus)
	if err := s.ScanAll(ctx); err != nil {
		t.Fatal(err)
	}

	// 7. Verify the media item exists, has the OLD UUID, and is linked properly
	item, err := sqlite.GetMediaItemByPath(ctx, db, moviePath)
	if err != nil {
		t.Fatal(err)
	}

	if item.ID != oldUUID {
		t.Errorf("expected media item ID to remain %s, got %s", oldUUID, item.ID)
	}
	if item.SourceStatus != models.SourceStatusAvailable {
		t.Errorf("SourceStatus: got %s, want available", item.SourceStatus)
	}
	if item.BundleStatus != models.BundleStatusAvailable {
		t.Errorf("BundleStatus: got %s, want available", item.BundleStatus)
	}
	if item.TranscodeStatus != models.TranscodeStatusDone {
		t.Errorf("TranscodeStatus: got %s, want done", item.TranscodeStatus)
	}

	expectedMPDPath := filepath.Join(outputDir, "manifest.mpd")
	if item.MPDPath == nil || *item.MPDPath != expectedMPDPath {
		got := "<nil>"
		if item.MPDPath != nil {
			got = *item.MPDPath
		}
		t.Errorf("MPDPath: got %s, want %s", got, expectedMPDPath)
	}

	// 8. Verify the artifact media link was successfully inserted in the DB (no foreign key error)
	links, err := sqlite.GetArtifactMediaLinkByMedia(ctx, db, item.ID)
	if err != nil {
		t.Fatalf("failed to query artifact media links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 artifact media link, got %d", len(links))
	}
	if links[0].MediaItemID != oldUUID {
		t.Errorf("expected link media_item_id to be %s, got %s", oldUUID, links[0].MediaItemID)
	}
}

func TestIntegration_ScanMatchesDiskBundle(t *testing.T) {
	ctx := context.Background()
	db, libraryDir, storageDir := setupTestEnv(t)

	// 1. Create a library & storage area
	lib := &models.Library{
		Path:      libraryDir,
		MediaType: models.MediaTypeMovie,
	}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	area := &models.StorageArea{
		Kind:    models.StorageAreaKindSegments,
		Path:    storageDir,
		Enabled: true,
	}
	if err := sqlite.CreateStorageArea(ctx, db, area); err != nil {
		t.Fatal(err)
	}

	// 2. Create the movie file in the library
	moviePath := filepath.Join(libraryDir, "disk-matching-movie.mkv")
	movieContent := []byte("disk-matching-movie-content")
	if err := os.WriteFile(moviePath, movieContent, 0644); err != nil {
		t.Fatal(err)
	}

	fp, err := fingerprint.GenerateDeterministic(moviePath)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Create the bundle directory and write artifact.json directly on disk.
	// Do NOT register it in the DB (simulating a database loss/reset where the DB doesn't have it).
	bundleID := uuid.New().String()
	outputDir := filepath.Join(storageDir, bundleID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "manifest.mpd"), []byte("<MPD/>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "media_0.mp4"), []byte("segments"), 0644); err != nil {
		t.Fatal(err)
	}

	sc := &artifact.SidecarMetadata{
		Version:           2,
		MediaItemID:       bundleID,
		SourcePath:        moviePath,
		SourceFingerprint: fp,
		OutputDir:         outputDir,
		MPDPath:           "manifest.mpd",
	}
	if err := artifact.WriteSidecar(outputDir, sc); err != nil {
		t.Fatal(err)
	}

	// 4. Scan the library. The scanner should:
	//    a. Discover the movie file.
	//    b. Realize there's no matching artifact in DB.
	//    c. Search segment storage on disk, find the matching sidecar by fingerprint.
	//    d. Automatically register the artifact record in the DB.
	//    e. Link it to the media item.
	bus := events.NewBus()
	s := New(db, lib, nil, bus)
	if err := s.ScanAll(ctx); err != nil {
		t.Fatal(err)
	}

	// 5. Verify the media item is created and linked correctly
	item, err := sqlite.GetMediaItemByPath(ctx, db, moviePath)
	if err != nil {
		t.Fatal(err)
	}

	if item.SourceStatus != models.SourceStatusAvailable {
		t.Errorf("SourceStatus: got %s, want available", item.SourceStatus)
	}
	if item.BundleStatus != models.BundleStatusAvailable {
		t.Errorf("BundleStatus: got %s, want available", item.BundleStatus)
	}
	if item.TranscodeStatus != models.TranscodeStatusDone {
		t.Errorf("TranscodeStatus: got %s, want done", item.TranscodeStatus)
	}

	expectedMPDPath := filepath.Join(outputDir, "manifest.mpd")
	if item.MPDPath == nil || *item.MPDPath != expectedMPDPath {
		got := "<nil>"
		if item.MPDPath != nil {
			got = *item.MPDPath
		}
		t.Errorf("MPDPath: got %s, want %s", got, expectedMPDPath)
	}

	// 6. Verify that the artifact record was created in the DB!
	artRecords, err := sqlite.ListArtifactRecordsByFingerprint(ctx, db, fp)
	if err != nil {
		t.Fatalf("failed to query artifact records by fingerprint: %v", err)
	}
	if len(artRecords) != 1 {
		t.Fatalf("expected 1 artifact record in DB, got %d", len(artRecords))
	}
	if artRecords[0].OutputDir != outputDir {
		t.Errorf("expected output dir %s, got %s", outputDir, artRecords[0].OutputDir)
	}

	// 7. Verify the link is also created
	links, err := sqlite.GetArtifactMediaLinkByMedia(ctx, db, item.ID)
	if err != nil {
		t.Fatalf("failed to query artifact media links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].ArtifactID != artRecords[0].ID {
		t.Errorf("expected link artifact ID to match registered record, got %s vs %s", links[0].ArtifactID, artRecords[0].ID)
	}
}
