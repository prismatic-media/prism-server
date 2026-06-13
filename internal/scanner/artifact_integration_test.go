package scanner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ringmaster217/prism/internal/artifact"
	"github.com/ringmaster217/prism/internal/models"
	"github.com/ringmaster217/prism/internal/store/sqlite"
	"github.com/ringmaster217/prism/pkg/events"
)

// TestIndexer_IndexStorageArea tests the full indexing flow:
// 1. Create a storage area with preexisting transcode output
// 2. Run the indexer
// 3. Verify artifact records are created
func TestIndexer_IndexStorageArea(t *testing.T) {
	ctx := context.Background()

	// Create a temporary storage area with a transcode output.
	storageDir := t.TempDir()
	outputDir := filepath.Join(storageDir, "test-media-id")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create MPD file.
	mpdPath := filepath.Join(outputDir, "manifest.mpd")
	if err := os.WriteFile(mpdPath, []byte("<MPD></MPD>"), 0644); err != nil {
		t.Fatalf("WriteFile MPD: %v", err)
	}

	// Create segment file.
	segPath := filepath.Join(outputDir, "media_0001.mp4")
	if err := os.WriteFile(segPath, []byte("segment"), 0644); err != nil {
		t.Fatalf("WriteFile segment: %v", err)
	}

	// Create sidecar.
	writtenAt := time.Now().UTC()
	profiles := []artifact.RenditionInfo{
		{Name: "720p", Height: 720, Width: 1280, VideoBitrateK: 4000, AudioBitrateK: 128},
	}
	sc := &artifact.SidecarMetadata{
		Version:             1,
		MediaItemID:         "test-media-id",
		SourcePath:          "videos/test.mp4",
		SourceFingerprint:   "abc123",
		OutputDir:           outputDir,
		MPDPath:             "manifest.mpd",
		Profiles:            profiles,
		Duration:            3600.5,
		WrittenAt:           writtenAt,
	}
	if err := artifact.WriteSidecar(outputDir, sc); err != nil {
		t.Fatalf("WriteSidecar: %v", err)
	}

	// Create a test database.
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Migrate.
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Create a storage area record.
	area := &models.StorageArea{
		Kind:    models.StorageAreaKindSegments,
		Path:    storageDir,
		Enabled: true,
	}
	if err := sqlite.CreateStorageArea(ctx, db, area); err != nil {
		t.Fatalf("CreateStorageArea: %v", err)
	}

	// Create the indexer.
	bus := events.NewBus()
	indexer := NewIndexer(db, bus)

	// Index the storage area.
	sum, err := indexer.IndexStorageArea(ctx, area)
	if err != nil {
		t.Fatalf("IndexStorageArea: %v", err)
	}

	if sum.Registered != 1 {
		t.Errorf("Registered: got %d, want 1", sum.Registered)
	}
	if sum.Updated != 0 {
		t.Errorf("Updated: got %d, want 0", sum.Updated)
	}
	if sum.Errors != 0 {
		t.Errorf("Errors: got %d, want 0", sum.Errors)
	}

	// Verify the artifact record exists.
	records, err := sqlite.ListArtifactRecordsByStorageArea(ctx, db, area.ID)
	if err != nil {
		t.Fatalf("ListArtifactRecordsByStorageArea: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("ListArtifactRecordsByStorageArea: got %d records, want 1", len(records))
	}

	rec := records[0]
	if rec.SourcePath != sc.SourcePath {
		t.Errorf("SourcePath: got %q, want %q", rec.SourcePath, sc.SourcePath)
	}
	if rec.OutputDir != outputDir {
		t.Errorf("OutputDir: got %q, want %q", rec.OutputDir, outputDir)
	}
	if rec.Health != models.ArtifactHealthHealthy {
		t.Errorf("Health: got %q, want %q", rec.Health, models.ArtifactHealthHealthy)
	}
}

// TestIndexer_IndexAll tests indexing all enabled storage areas.
func TestIndexer_IndexAll(t *testing.T) {
	ctx := context.Background()

	// Create two storage areas.
	storageDir1 := t.TempDir()
	storageDir2 := t.TempDir()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Create storage areas.
	area1 := &models.StorageArea{
		Kind:    models.StorageAreaKindSegments,
		Path:    storageDir1,
		Enabled: true,
	}
	area2 := &models.StorageArea{
		Kind:    models.StorageAreaKindSegments,
		Path:    storageDir2,
		Enabled: true,
	}
	if err := sqlite.CreateStorageArea(ctx, db, area1); err != nil {
		t.Fatalf("CreateStorageArea 1: %v", err)
	}
	if err := sqlite.CreateStorageArea(ctx, db, area2); err != nil {
		t.Fatalf("CreateStorageArea 2: %v", err)
	}

	bus := events.NewBus()
	indexer := NewIndexer(db, bus)

	// Index all (empty directories — no artifacts to find).
	summaries, err := indexer.IndexAll(ctx)
	if err != nil {
		t.Fatalf("IndexAll: %v", err)
	}

	if len(summaries) != 2 {
		t.Errorf("IndexAll: got %d summaries, want 2", len(summaries))
	}
}

// TestIndexer_IdempotentIndexing tests that re-indexing is idempotent.
func TestIndexer_IdempotentIndexing(t *testing.T) {
	ctx := context.Background()

	storageDir := t.TempDir()
	outputDir := filepath.Join(storageDir, "test-media-id")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create sidecar.
	writtenAt := time.Now().UTC()
	sc := &artifact.SidecarMetadata{
		Version:             1,
		MediaItemID:         "test-media-id",
		SourcePath:          "videos/test.mp4",
		SourceFingerprint:   "abc123",
		OutputDir:           outputDir,
		MPDPath:             "manifest.mpd",
		WrittenAt:           writtenAt,
	}
	if err := artifact.WriteSidecar(outputDir, sc); err != nil {
		t.Fatalf("WriteSidecar: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	area := &models.StorageArea{
		Kind:    models.StorageAreaKindSegments,
		Path:    storageDir,
		Enabled: true,
	}
	if err := sqlite.CreateStorageArea(ctx, db, area); err != nil {
		t.Fatalf("CreateStorageArea: %v", err)
	}

	bus := events.NewBus()
	indexer := NewIndexer(db, bus)

	// First index.
	sum1, err := indexer.IndexStorageArea(ctx, area)
	if err != nil {
		t.Fatalf("IndexStorageArea 1: %v", err)
	}
	if sum1.Registered != 1 {
		t.Errorf("First index Registered: got %d, want 1", sum1.Registered)
	}

	// Second index (should update, not register).
	sum2, err := indexer.IndexStorageArea(ctx, area)
	if err != nil {
		t.Fatalf("IndexStorageArea 2: %v", err)
	}
	if sum2.Updated != 1 {
		t.Errorf("Second index Updated: got %d, want 1", sum2.Updated)
	}
	if sum2.Registered != 0 {
		t.Errorf("Second index Registered: got %d, want 0", sum2.Registered)
	}

	// Verify only one record exists.
	records, err := sqlite.ListArtifactRecordsByStorageArea(ctx, db, area.ID)
	if err != nil {
		t.Fatalf("ListArtifactRecordsByStorageArea: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("ListArtifactRecordsByStorageArea: got %d records, want 1", len(records))
	}
}

// TestRelink_RelinkExact tests the exact fingerprint-based relinking flow.
func TestRelink_RelinkExact(t *testing.T) {
	ctx := context.Background()

	// Create a storage area with preexisting transcode output.
	storageDir := t.TempDir()
	outputDir := filepath.Join(storageDir, "test-media-id")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create MPD file.
	if err := os.WriteFile(filepath.Join(outputDir, "manifest.mpd"), []byte("<MPD></MPD>"), 0644); err != nil {
		t.Fatalf("WriteFile MPD: %v", err)
	}

	// Create segment file.
	if err := os.WriteFile(filepath.Join(outputDir, "media_0001.mp4"), []byte("segment"), 0644); err != nil {
		t.Fatalf("WriteFile segment: %v", err)
	}

	// Create sidecar with known fingerprint.
	writtenAt := time.Now().UTC()
	sc := &artifact.SidecarMetadata{
		Version:             1,
		MediaItemID:         "test-media-id",
		SourcePath:          "videos/test.mp4",
		SourceFingerprint:   "known-fingerprint",
		OutputDir:           outputDir,
		MPDPath:             "manifest.mpd",
		WrittenAt:           writtenAt,
	}
	if err := artifact.WriteSidecar(outputDir, sc); err != nil {
		t.Fatalf("WriteSidecar: %v", err)
	}

	// Create test database.
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Create storage area.
	area := &models.StorageArea{
		Kind:    models.StorageAreaKindSegments,
		Path:    storageDir,
		Enabled: true,
	}
	if err := sqlite.CreateStorageArea(ctx, db, area); err != nil {
		t.Fatalf("CreateStorageArea: %v", err)
	}

	// Create a library for the media item.
	library := &models.Library{
		Path:      "/test/library",
		MediaType: models.MediaTypeMovie,
	}
	if err := sqlite.CreateLibrary(ctx, db, library); err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}

	// Create a media item with matching fingerprint.
	mediaFilePath := filepath.Join(storageDir, "videos", "test.mp4")
	if err := os.MkdirAll(filepath.Dir(mediaFilePath), 0755); err != nil {
		t.Fatalf("MkdirAll source dir: %v", err)
	}
	if err := os.WriteFile(mediaFilePath, []byte("source-video-content"), 0644); err != nil {
		t.Fatalf("WriteFile source: %v", err)
	}
	mediaItem := &models.MediaItem{
		ID:              uuid.New(),
		LibraryID:       library.ID,
		Title:           "Test Video",
		MediaType:       models.MediaTypeMovie,
		FilePath:        filepath.Join(storageDir, "videos", "test.mp4"),
		FileSize:        1024,
		Duration:        3600.5,
		TranscodeStatus: models.TranscodeStatusDone,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := sqlite.UpsertMediaItem(ctx, db, mediaItem); err != nil {
		t.Fatalf("UpsertMediaItem: %v", err)
	}

	// Index the storage area to create artifact record.
	bus := events.NewBus()
	indexer := NewIndexer(db, bus)
	sum, err := indexer.IndexStorageArea(ctx, area)
	if err != nil {
		t.Fatalf("IndexStorageArea: %v", err)
	}
	if sum.Registered != 1 {
		t.Errorf("Registered: got %d, want 1", sum.Registered)
	}

	// Get the artifact record.
	records, err := sqlite.ListArtifactRecordsByStorageArea(ctx, db, area.ID)
	if err != nil {
		t.Fatalf("ListArtifactRecordsByStorageArea: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("ListArtifactRecordsByStorageArea: got %d records, want 1", len(records))
	}
	artifactID := records[0].ID

	// Verify no link exists yet.
	links, err := sqlite.GetArtifactMediaLinkByMedia(ctx, db, mediaItem.ID)
	if err != nil && !errors.Is(err, sqlite.ErrNotFound) {
		t.Fatalf("GetArtifactMediaLinkByMedia: %v", err)
	}
	if len(links) > 0 {
		t.Errorf("Expected no link before relink, got %d", len(links))
	}

	// Run exact relink.
	result, err := indexer.RelinkExact(ctx)
	if err != nil {
		t.Fatalf("RelinkExact: %v", err)
	}

	// Verify relink result.
	if result.Linked != 1 {
		t.Errorf("Linked: got %d, want 1", result.Linked)
	}
	if result.Unmatched != 0 {
		t.Errorf("Unmatched: got %d, want 0", result.Unmatched)
	}

	// Verify the link was created.
	links, err = sqlite.GetArtifactMediaLinkByMedia(ctx, db, mediaItem.ID)
	if err != nil {
		t.Fatalf("GetArtifactMediaLinkByMedia after relink: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("GetArtifactMediaLinkByMedia: got %d links, want 1", len(links))
	}
	link := links[0]
	if link.ArtifactID != artifactID {
		t.Errorf("ArtifactID: got %s, want %s", link.ArtifactID, artifactID)
	}
	if link.Status != models.ArtifactLinkLinked {
		t.Errorf("Status: got %q, want %q", link.Status, models.ArtifactLinkLinked)
	}
}

// TestRelink_RelinkExactUnmatched tests relinking when no matching media item exists.
func TestRelink_RelinkExactUnmatched(t *testing.T) {
	ctx := context.Background()

	storageDir := t.TempDir()
	outputDir := filepath.Join(storageDir, "orphan-media-id")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := os.WriteFile(filepath.Join(outputDir, "manifest.mpd"), []byte("<MPD></MPD>"), 0644); err != nil {
		t.Fatalf("WriteFile MPD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "media_0001.mp4"), []byte("segment"), 0644); err != nil {
		t.Fatalf("WriteFile segment: %v", err)
	}

	writtenAt := time.Now().UTC()
	sc := &artifact.SidecarMetadata{
		Version:             1,
		MediaItemID:         "orphan-media-id",
		SourcePath:          "videos/orphan.mp4",
		SourceFingerprint:   "unique-fingerprint",
		OutputDir:           outputDir,
		MPDPath:             "manifest.mpd",
		WrittenAt:           writtenAt,
	}
	if err := artifact.WriteSidecar(outputDir, sc); err != nil {
		t.Fatalf("WriteSidecar: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	area := &models.StorageArea{
		Kind:    models.StorageAreaKindSegments,
		Path:    storageDir,
		Enabled: true,
	}
	if err := sqlite.CreateStorageArea(ctx, db, area); err != nil {
		t.Fatalf("CreateStorageArea: %v", err)
	}

	// Note: No media item is created — this artifact is orphaned.
	bus := events.NewBus()
	indexer := NewIndexer(db, bus)
	sum, err := indexer.IndexStorageArea(ctx, area)
	if err != nil {
		t.Fatalf("IndexStorageArea: %v", err)
	}
	if sum.Registered != 1 {
		t.Errorf("Registered: got %d, want 1", sum.Registered)
	}

	// Run relink.
	result, err := indexer.RelinkExact(ctx)
	if err != nil {
		t.Fatalf("RelinkExact: %v", err)
	}

	// The artifact should remain unmatched.
	if result.Linked != 0 {
		t.Errorf("Linked: got %d, want 0", result.Linked)
	}
	if result.Unmatched != 1 {
		t.Errorf("Unmatched: got %d, want 1", result.Unmatched)
	}
}

// TestIndexer_DatabaseLossRecovery simulates a database loss scenario:
// 1. Index artifacts from disk (simulate fresh start after DB loss)
// 2. Verify all artifacts are recovered
func TestIndexer_DatabaseLossRecovery(t *testing.T) {
	ctx := context.Background()

	// Create a storage area with multiple transcode outputs.
	storageDir := t.TempDir()
	outputDirs := []string{
		filepath.Join(storageDir, "media-1"),
		filepath.Join(storageDir, "media-2"),
		filepath.Join(storageDir, "media-3"),
	}

	for i, outputDir := range outputDirs {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		// Create MPD file.
		if err := os.WriteFile(filepath.Join(outputDir, "manifest.mpd"), []byte("<MPD></MPD>"), 0644); err != nil {
			t.Fatalf("WriteFile MPD: %v", err)
		}
		// Create segment file.
		if err := os.WriteFile(filepath.Join(outputDir, "media_0001.mp4"), []byte("segment"), 0644); err != nil {
			t.Fatalf("WriteFile segment: %v", err)
		}
		// Create artifact sidecar.
		writtenAt := time.Now().UTC()
		sc := &artifact.SidecarMetadata{
			Version:             1,
			MediaItemID:         fmt.Sprintf("media-%d", i+1),
			SourcePath:          fmt.Sprintf("videos/file-%d.mp4", i+1),
			SourceFingerprint:   fmt.Sprintf("fingerprint-%d", i+1),
			OutputDir:           outputDir,
			MPDPath:             "manifest.mpd",
			WrittenAt:           writtenAt,
		}
		if err := artifact.WriteSidecar(outputDir, sc); err != nil {
			t.Fatalf("WriteSidecar: %v", err)
		}
	}

	// Simulate database loss: create a fresh empty database.
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Create storage area.
	area := &models.StorageArea{
		Kind:    models.StorageAreaKindSegments,
		Path:    storageDir,
		Enabled: true,
	}
	if err := sqlite.CreateStorageArea(ctx, db, area); err != nil {
		t.Fatalf("CreateStorageArea: %v", err)
	}

	// Verify no artifacts exist yet.
	healthCounts, err := sqlite.CountArtifactRecordsByHealth(ctx, db, area.ID)
	if err != nil {
		t.Fatalf("CountArtifactRecordsByHealth: %v", err)
	}
	healthyCount := 0
	for _, hc := range healthCounts {
		if hc.Health == models.ArtifactHealthHealthy {
			healthyCount = hc.Count
		}
	}
	if healthyCount != 0 {
		t.Errorf("Expected 0 healthy artifacts before index, got %d", healthyCount)
	}

	// Simulate database loss recovery: run the indexer.
	bus := events.NewBus()
	indexer := NewIndexer(db, bus)
	summaries, err := indexer.IndexAll(ctx)
	if err != nil {
		t.Fatalf("IndexAll: %v", err)
	}

	// Verify artifacts were recovered from disk.
	if len(summaries) != 1 {
		t.Fatalf("IndexAll: got %d summaries, want 1", len(summaries))
	}
	if summaries[0].Registered != 3 {
		t.Errorf("Registered: got %d, want 3", summaries[0].Registered)
	}

	// Verify all 3 artifacts exist in the database.
	records, err := sqlite.ListArtifactRecordsByStorageArea(ctx, db, area.ID)
	if err != nil {
		t.Fatalf("ListArtifactRecordsByStorageArea: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("ListArtifactRecordsByStorageArea: got %d records, want 3", len(records))
	}

	// Verify all are healthy.
	for _, rec := range records {
		if rec.Health != models.ArtifactHealthHealthy {
			t.Errorf("Artifact %s health: got %q, want %q", rec.ID, rec.Health, models.ArtifactHealthHealthy)
		}
	}
}

