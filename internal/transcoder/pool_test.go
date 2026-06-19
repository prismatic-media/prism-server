package transcoder

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/artifact"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/pkg/dash"
	"github.com/prismatic-media/prism-server/pkg/events"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("sqlite.Migrate: %v", err)
	}
	if err := sqlite.BootstrapSettings(context.Background(), db); err != nil {
		t.Fatalf("sqlite.BootstrapSettings: %v", err)
	}
	return db
}

func seedMovie(t *testing.T, db *sql.DB, path string) *models.MediaItem {
	t.Helper()
	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	m := &models.MediaItem{
		LibraryID:       lib.ID,
		Title:           "Movie",
		MediaType:       models.MediaTypeMovie,
		FilePath:        path,
		TranscodeStatus: models.TranscodeStatusNone,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatalf("UpsertMediaItem: %v", err)
	}
	return m
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func TestPoolStart_RecoversStaleProcessingJobs(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := seedMovie(t, db, "/l/recover.mkv")
	j := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, j); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobStatus(ctx, db, j.ID, models.TranscodeStatusProcessing, nil); err != nil {
		t.Fatal(err)
	}

	pool := NewPool(db, 0, &dash.Cache{}, events.NewBus())
	if err := pool.Start(ctx); err != nil {
		t.Fatal(err)
	}
	pool.Stop()

	got, err := sqlite.GetTranscodeJobByID(ctx, db, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.TranscodeStatusPending {
		t.Fatalf("status = %q, want pending", got.Status)
	}
}

func TestPoolAutoEnqueueOnDiscovery_Enabled(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sqlite.SetSetting(ctx, db, "auto_transcode_on_discovery", "true"); err != nil {
		t.Fatal(err)
	}

	m := seedMovie(t, db, "/l/auto-enabled.mkv")
	bus := events.NewBus()
	pool := NewPool(db, 0, &dash.Cache{}, bus)
	if err := pool.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer pool.Stop()

	waitFor(t, 2*time.Second, func() bool {
		bus.Publish(events.EventMediaCreated, events.MediaCreatedPayload{MediaItemID: m.ID, LibraryID: m.LibraryID, Title: m.Title})
		has, err := sqlite.HasTranscodeJobForMediaItem(ctx, db, m.ID)
		return err == nil && has
	})
}

func TestPoolAutoEnqueueOnDiscovery_Disabled(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sqlite.SetSetting(ctx, db, "auto_transcode_on_discovery", "false"); err != nil {
		t.Fatal(err)
	}

	m := seedMovie(t, db, "/l/auto-disabled.mkv")
	bus := events.NewBus()
	pool := NewPool(db, 0, &dash.Cache{}, bus)
	if err := pool.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer pool.Stop()

	bus.Publish(events.EventMediaCreated, events.MediaCreatedPayload{MediaItemID: m.ID, LibraryID: m.LibraryID, Title: m.Title})
	time.Sleep(200 * time.Millisecond)

	has, err := sqlite.HasTranscodeJobForMediaItem(ctx, db, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected no transcode job when auto enqueue disabled")
	}
}

func TestPoolAutoEnqueueOnDiscovery_Idempotent(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sqlite.SetSetting(ctx, db, "auto_transcode_on_discovery", "true"); err != nil {
		t.Fatal(err)
	}

	m := seedMovie(t, db, "/l/auto-idempotent.mkv")
	existing := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, existing); err != nil {
		t.Fatal(err)
	}

	bus := events.NewBus()
	pool := NewPool(db, 0, &dash.Cache{}, bus)
	if err := pool.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer pool.Stop()

	bus.Publish(events.EventMediaCreated, events.MediaCreatedPayload{MediaItemID: m.ID, LibraryID: m.LibraryID, Title: m.Title})
	time.Sleep(200 * time.Millisecond)

	jobs, err := sqlite.ListTranscodeJobs(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
}

func TestPoolRunWorker_PrioritizedRunsFirst(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sqlite.SetSetting(ctx, db, "transcode_poll_interval", "1"); err != nil {
		t.Fatal(err)
	}

	// Use distinct library paths to satisfy unique constraint.
	lib1 := &models.Library{Path: "/l1", MediaType: models.MediaTypeMovie}
	lib2 := &models.Library{Path: "/l2", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(ctx, db, lib1); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.CreateLibrary(ctx, db, lib2); err != nil {
		t.Fatal(err)
	}
	m1 := &models.MediaItem{LibraryID: lib1.ID, Title: "A", MediaType: models.MediaTypeMovie, FilePath: "/l1/a.mkv", TranscodeStatus: models.TranscodeStatusNone}
	m2 := &models.MediaItem{LibraryID: lib2.ID, Title: "B", MediaType: models.MediaTypeMovie, FilePath: "/l2/b.mkv", TranscodeStatus: models.TranscodeStatusNone}
	if err := sqlite.UpsertMediaItem(ctx, db, m1); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpsertMediaItem(ctx, db, m2); err != nil {
		t.Fatal(err)
	}

	j1 := &models.TranscodeJob{MediaItemID: m1.ID}
	j2 := &models.TranscodeJob{MediaItemID: m2.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, j1); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.CreateTranscodeJob(ctx, db, j2); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.PrioritizeJob(ctx, db, j2.ID); err != nil {
		t.Fatal(err)
	}

	pool := NewPool(db, 1, &dash.Cache{}, events.NewBus())
	if err := pool.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer pool.Stop()

	waitFor(t, 4*time.Second, func() bool {
		g1, err1 := sqlite.GetTranscodeJobByID(ctx, db, j1.ID)
		g2, err2 := sqlite.GetTranscodeJobByID(ctx, db, j2.ID)
		if err1 != nil || err2 != nil {
			return false
		}
		return g1.StartedAt != nil && g2.StartedAt != nil
	})

	g1, _ := sqlite.GetTranscodeJobByID(ctx, db, j1.ID)
	g2, _ := sqlite.GetTranscodeJobByID(ctx, db, j2.ID)
	if g2.StartedAt.After(*g1.StartedAt) {
		t.Fatalf("prioritized job started after normal job: prioritized=%s normal=%s", g2.StartedAt.String(), g1.StartedAt.String())
	}
}

func TestPoolLoadPollInterval_Parsing(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	pool := NewPool(db, 0, &dash.Cache{}, events.NewBus())

	if err := sqlite.SetSetting(ctx, db, "transcode_poll_interval", "5"); err != nil {
		t.Fatal(err)
	}
	if got := pool.loadPollInterval(ctx); got != 5*time.Second {
		t.Fatalf("poll interval = %v, want 5s", got)
	}

	if err := sqlite.SetSetting(ctx, db, "transcode_poll_interval", "bad"); err != nil {
		t.Fatal(err)
	}
	if got := pool.loadPollInterval(ctx); got != 15*time.Second {
		t.Fatalf("poll interval = %v, want default 15s", got)
	}
}

func TestPoolEnqueue_UpdatesMediaStatusPending(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := &models.Library{Path: "/l3", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := &models.MediaItem{LibraryID: lib.ID, Title: "M", MediaType: models.MediaTypeMovie, FilePath: "/l3/m.mkv", TranscodeStatus: models.TranscodeStatusDone}
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	pool := NewPool(db, 0, &dash.Cache{}, events.NewBus())
	if _, err := pool.Enqueue(ctx, m.ID, false); err != nil {
		t.Fatal(err)
	}

	got, err := sqlite.GetMediaItemByID(ctx, db, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TranscodeStatus != models.TranscodeStatusPending {
		t.Fatalf("transcode status = %q, want pending", got.TranscodeStatus)
	}

	has, err := sqlite.HasTranscodeJobForMediaItem(ctx, db, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected transcode job to exist after enqueue")
	}
}

func TestPoolPrioritizeEndpointFlowClaimsTargetFirst(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := &models.Library{Path: "/l4", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m1 := &models.MediaItem{ID: uuid.New(), LibraryID: lib.ID, Title: "A", MediaType: models.MediaTypeMovie, FilePath: "/l4/a.mkv", TranscodeStatus: models.TranscodeStatusNone}
	m2 := &models.MediaItem{ID: uuid.New(), LibraryID: lib.ID, Title: "B", MediaType: models.MediaTypeMovie, FilePath: "/l4/b.mkv", TranscodeStatus: models.TranscodeStatusNone}
	if err := sqlite.UpsertMediaItem(ctx, db, m1); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpsertMediaItem(ctx, db, m2); err != nil {
		t.Fatal(err)
	}
	j1 := &models.TranscodeJob{MediaItemID: m1.ID}
	j2 := &models.TranscodeJob{MediaItemID: m2.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, j1); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.CreateTranscodeJob(ctx, db, j2); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.PrioritizeJob(ctx, db, j2.ID); err != nil {
		t.Fatal(err)
	}

	claimed, err := sqlite.ClaimNextJob(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.ID != j2.ID {
		t.Fatalf("expected prioritized job %s to be claimed first, got %+v", j2.ID, claimed)
	}
}

func TestSelectSegmentsOutputDir_SkipsInvalidAndChoosesEligible(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seeded, err := sqlite.ListStorageAreasByKind(ctx, db, models.StorageAreaKindSegments, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, area := range seeded {
		if err := sqlite.UpdateStorageArea(ctx, db, area.ID, area.Path, false); err != nil {
			t.Fatal(err)
		}
	}

	if err := sqlite.SetSetting(ctx, db, "storage_min_free_bytes", "0"); err != nil {
		t.Fatal(err)
	}

	valid := t.TempDir()
	if err := sqlite.CreateStorageArea(ctx, db, &models.StorageArea{Kind: models.StorageAreaKindSegments, Path: valid, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.CreateStorageArea(ctx, db, &models.StorageArea{Kind: models.StorageAreaKindSegments, Path: "/definitely/missing/path", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	pool := NewPool(db, 0, &dash.Cache{}, events.NewBus())
	mediaID := uuid.New()
	out, err := pool.SelectSegmentsOutputDir(ctx, mediaID)
	if err != nil {
		t.Fatalf("selectSegmentsOutputDir: %v", err)
	}
	if !strings.HasPrefix(out, valid) {
		t.Fatalf("output dir %q should be under %q", out, valid)
	}
	if filepath.Base(out) != mediaID.String() {
		t.Fatalf("output dir %q should end with media id %s", out, mediaID)
	}
}

func TestSelectSegmentsOutputDir_NoEligibleAreaFails(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seeded, err := sqlite.ListStorageAreasByKind(ctx, db, models.StorageAreaKindSegments, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, area := range seeded {
		if err := sqlite.UpdateStorageArea(ctx, db, area.ID, area.Path, false); err != nil {
			t.Fatal(err)
		}
	}

	valid := t.TempDir()
	if err := sqlite.CreateStorageArea(ctx, db, &models.StorageArea{Kind: models.StorageAreaKindSegments, Path: valid, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.SetSetting(ctx, db, "storage_min_free_bytes", "999999999999999999"); err != nil {
		t.Fatal(err)
	}

	pool := NewPool(db, 0, &dash.Cache{}, events.NewBus())
	_, err = pool.SelectSegmentsOutputDir(ctx, uuid.New())
	if err == nil {
		t.Fatal("expected error when no area is above reserve threshold")
	}
	if !strings.Contains(err.Error(), "no eligible segment storage area") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteSidecarForMediaItem(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// 1. Create a dummy source file
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "source.mp4")
	if err := os.WriteFile(sourcePath, []byte("dummy file content for fingerprinting"), 0644); err != nil {
		t.Fatalf("failed to write dummy source file: %v", err)
	}

	// 2. Create a dummy output directory and existing sidecar
	outputDir := t.TempDir()
	existingSidecar := filepath.Join(outputDir, "artifact.json")
	existingData := `{
		"v": 2,
		"media_item_id": "00000000-0000-0000-0000-000000000000",
		"profiles": [
			{
				"name": "1080p",
				"height": 1080,
				"width": 1920,
				"video_bitrate_k": 4000,
				"audio_bitrate_k": 192
			}
		]
	}`
	if err := os.WriteFile(existingSidecar, []byte(existingData), 0644); err != nil {
		t.Fatalf("failed to write existing sidecar: %v", err)
	}

	// 3. Seed library and media item
	lib := &models.Library{Path: sourceDir, MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	mpdPath := filepath.Join(outputDir, "manifest.mpd")
	m := &models.MediaItem{
		LibraryID:       lib.ID,
		Title:           "Original Title",
		MediaType:       models.MediaTypeMovie,
		FilePath:        sourcePath,
		TranscodeStatus: models.TranscodeStatusDone,
		MPDPath:         &mpdPath,
		Duration:        123.45,
	}
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	// 4. Call WriteSidecarForMediaItem
	if err := WriteSidecarForMediaItem(ctx, db, m.ID); err != nil {
		t.Fatalf("WriteSidecarForMediaItem failed: %v", err)
	}

	// 5. Read and parse the regenerated sidecar
	meta, err := artifact.ReadSidecar(outputDir)
	if err != nil {
		t.Fatalf("failed to read sidecar: %v", err)
	}

	if meta.Version != 2 {
		t.Errorf("expected version 2, got %d", meta.Version)
	}
	if meta.Duration != 123.45 {
		t.Errorf("expected duration 123.45, got %f", meta.Duration)
	}
	if len(meta.Profiles) != 1 || meta.Profiles[0].Name != "1080p" {
		t.Errorf("profiles not preserved: %+v", meta.Profiles)
	}
}

func TestPoolEnqueue_ReuseAndResetJob(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := &models.Library{Path: "/l5", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := &models.MediaItem{LibraryID: lib.ID, Title: "M", MediaType: models.MediaTypeMovie, FilePath: "/l5/m.mkv", TranscodeStatus: models.TranscodeStatusFailed}
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	// Create a failed job first
	j := &models.TranscodeJob{MediaItemID: m.ID, Priority: 8}
	if err := sqlite.CreateTranscodeJob(ctx, db, j); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobStatus(ctx, db, j.ID, models.TranscodeStatusFailed, nil); err != nil {
		t.Fatal(err)
	}

	// Set priority to check if preserved
	_, err := db.ExecContext(ctx, `UPDATE transcode_jobs SET priority = 8 WHERE id = ?`, j.ID.String())
	if err != nil {
		t.Fatal(err)
	}

	pool := NewPool(db, 0, &dash.Cache{}, events.NewBus())
	reusedJob, err := pool.Enqueue(ctx, m.ID, false)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	if reusedJob.ID != j.ID {
		t.Errorf("expected job ID to be reused: got %v, want %v", reusedJob.ID, j.ID)
	}
	if reusedJob.Status != models.TranscodeStatusPending {
		t.Errorf("reused job status = %q, want pending", reusedJob.Status)
	}
	if reusedJob.Priority != 8 {
		t.Errorf("reused job priority = %d, want 8", reusedJob.Priority)
	}
}

func TestPoolEnqueue_ActiveJobReturnsAsIs(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := &models.Library{Path: "/l6", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := &models.MediaItem{LibraryID: lib.ID, Title: "M", MediaType: models.MediaTypeMovie, FilePath: "/l6/m.mkv", TranscodeStatus: models.TranscodeStatusProcessing}
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	// Create a processing job first
	j := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, j); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobStatus(ctx, db, j.ID, models.TranscodeStatusProcessing, nil); err != nil {
		t.Fatal(err)
	}

	pool := NewPool(db, 0, &dash.Cache{}, events.NewBus())
	reusedJob, err := pool.Enqueue(ctx, m.ID, false)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	if reusedJob.ID != j.ID {
		t.Errorf("expected job ID to be reused: got %v, want %v", reusedJob.ID, j.ID)
	}
	if reusedJob.Status != models.TranscodeStatusProcessing {
		t.Errorf("reused job status = %q, want processing (should not have been reset)", reusedJob.Status)
	}
}


