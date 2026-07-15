package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)


func TestCreateTranscodeJob(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "Film", "/l/film.mkv")
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}

	j := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, j); err != nil {
		t.Fatalf("CreateTranscodeJob: %v", err)
	}
	if j.ID == uuid.Nil {
		t.Error("expected job ID to be set")
	}
	if j.Status != models.TranscodeStatusPending {
		t.Errorf("status = %q, want pending", j.Status)
	}
}

func TestGetTranscodeJobByID_Found(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "Film", "/l/film.mkv")
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}
	j := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, j); err != nil {
		t.Fatal(err)
	}

	got, err := sqlite.GetTranscodeJobByID(context.Background(), db, j.ID)
	if err != nil {
		t.Fatalf("GetTranscodeJobByID: %v", err)
	}
	if got.MediaItemID != m.ID {
		t.Errorf("MediaItemID mismatch")
	}
}

func TestGetTranscodeJobByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := sqlite.GetTranscodeJobByID(context.Background(), db, uuid.New())
	if err == nil {
		t.Error("expected error for missing job")
	}
}

func TestListTranscodeJobs(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	for i, title := range []string{"A", "B", "C"} {
		m := newMovieItem(lib.ID, title, "/l/"+title+".mkv")
		if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
			t.Fatal(err)
		}
		j := &models.TranscodeJob{MediaItemID: m.ID}
		if err := sqlite.CreateTranscodeJob(context.Background(), db, j); err != nil {
			t.Fatalf("job %d: %v", i, err)
		}
	}

	jobs, err := sqlite.ListTranscodeJobs(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Errorf("want 3 jobs, got %d", len(jobs))
	}
}

func TestListPendingJobs(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}

	pending := newMovieItem(lib.ID, "Pending", "/l/pending.mkv")
	done := newMovieItem(lib.ID, "Done", "/l/done.mkv")
	for _, m := range []*models.MediaItem{pending, done} {
		if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
			t.Fatal(err)
		}
		j := &models.TranscodeJob{MediaItemID: m.ID}
		if err := sqlite.CreateTranscodeJob(context.Background(), db, j); err != nil {
			t.Fatal(err)
		}
		if m == done {
			if err := sqlite.UpdateJobStatus(context.Background(), db, j.ID, models.TranscodeStatusDone, nil); err != nil {
				t.Fatal(err)
			}
		}
	}

	jobs, err := sqlite.ListPendingJobs(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Errorf("want 1 pending job, got %d", len(jobs))
	}
}

func TestUpdateJobStatus_Processing(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "F", "/l/f.mkv")
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}
	j := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, j); err != nil {
		t.Fatal(err)
	}

	if err := sqlite.UpdateJobStatus(context.Background(), db, j.ID, models.TranscodeStatusProcessing, nil); err != nil {
		t.Fatalf("UpdateJobStatus: %v", err)
	}

	got, _ := sqlite.GetTranscodeJobByID(context.Background(), db, j.ID)
	if got.Status != models.TranscodeStatusProcessing {
		t.Errorf("status = %q, want processing", got.Status)
	}
	if got.StartedAt == nil {
		t.Error("started_at should be set for processing status")
	}
}

func TestUpdateJobStatus_Failed(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "F", "/l/f2.mkv")
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}
	j := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, j); err != nil {
		t.Fatal(err)
	}

	errMsg := "ffmpeg crashed"
	if err := sqlite.UpdateJobStatus(context.Background(), db, j.ID, models.TranscodeStatusFailed, &errMsg); err != nil {
		t.Fatalf("UpdateJobStatus: %v", err)
	}

	got, _ := sqlite.GetTranscodeJobByID(context.Background(), db, j.ID)
	if got.Status != models.TranscodeStatusFailed {
		t.Errorf("status = %q, want failed", got.Status)
	}
	if got.ErrorMsg == nil || *got.ErrorMsg != errMsg {
		t.Errorf("error_msg = %v, want %q", got.ErrorMsg, errMsg)
	}
	if got.FinishedAt == nil {
		t.Error("finished_at should be set for failed status")
	}
}

func TestSetMediaMPDPath(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "F", "/l/f3.mkv")
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}

	const mpdPath = "/data/segments/abc/manifest.mpd"
	if err := sqlite.SetMediaMPDPath(context.Background(), db, m.ID, mpdPath); err != nil {
		t.Fatalf("SetMediaMPDPath: %v", err)
	}

	got, _ := sqlite.GetMediaItemByID(context.Background(), db, m.ID)
	if got.MPDPath == nil || *got.MPDPath != mpdPath {
		t.Errorf("mpd_path = %v, want %q", got.MPDPath, mpdPath)
	}
	if got.TranscodeStatus != models.TranscodeStatusDone {
		t.Errorf("transcode_status = %q, want done", got.TranscodeStatus)
	}
}

func TestUpdateJobProgress(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "F", "/l/f4.mkv")
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}
	j := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, j); err != nil {
		t.Fatal(err)
	}

	if err := sqlite.UpdateJobProgress(context.Background(), db, j.ID, 42.5); err != nil {
		t.Fatalf("UpdateJobProgress: %v", err)
	}

	got, _ := sqlite.GetTranscodeJobByID(context.Background(), db, j.ID)
	if got.Progress != 42.5 {
		t.Errorf("progress = %v, want 42.5", got.Progress)
	}
}

func TestClaimNextSubJob_RespectsPriorityThenFIFO(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	m1 := newMovieItem(lib.ID, "A", "/l/a.mkv")
	m2 := newMovieItem(lib.ID, "B", "/l/b.mkv")
	for _, m := range []*models.MediaItem{m1, m2} {
		if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
			t.Fatal(err)
		}
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

	// Claim all sub-jobs for j2 (which has 5 sub-jobs)
	for i := 0; i < 5; i++ {
		claimed, err := sqlite.ClaimNextSubJob(ctx, db, nil)
		if err != nil {
			t.Fatal(err)
		}
		if claimed == nil || claimed.JobID != j2.ID {
			t.Fatalf("expected claim %d to be sub-job of prioritized job %s, got %+v", i+1, j2.ID, claimed)
		}
	}

	// The 6th claim should be from j1
	claimed6, err := sqlite.ClaimNextSubJob(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if claimed6 == nil || claimed6.JobID != j1.ID {
		t.Fatalf("expected 6th claim to be remaining job %s, got %+v", j1.ID, claimed6)
	}
}

func TestRecoverStaleJobs(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "Movie", "/l/movie.mkv")
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}
	j := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, j); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobStatus(ctx, db, j.ID, models.TranscodeStatusProcessing, nil); err != nil {
		t.Fatal(err)
	}

	if err := sqlite.RecoverStaleJobs(ctx, db); err != nil {
		t.Fatal(err)
	}

	got, err := sqlite.GetTranscodeJobByID(ctx, db, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.TranscodeStatusPending {
		t.Fatalf("status = %q, want pending", got.Status)
	}
}

func TestPrioritizeJob_NotPending(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "Movie", "/l/movie2.mkv")
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}
	j := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, j); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobStatus(ctx, db, j.ID, models.TranscodeStatusDone, nil); err != nil {
		t.Fatal(err)
	}

	err := sqlite.PrioritizeJob(ctx, db, j.ID)
	if !errors.Is(err, sqlite.ErrJobNotPending) {
		t.Fatalf("expected ErrJobNotPending, got %v", err)
	}
}

func TestBulkEnqueueUntranscoded(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	m1 := newMovieItem(lib.ID, "A", "/l/u1.mkv")
	m2 := newMovieItem(lib.ID, "B", "/l/u2.mkv")
	for _, m := range []*models.MediaItem{m1, m2} {
		if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
			t.Fatal(err)
		}
	}

	// Seed one pre-existing job so only one media item is untouched.
	j := &models.TranscodeJob{MediaItemID: m1.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, j); err != nil {
		t.Fatal(err)
	}

	n, err := sqlite.BulkEnqueueUntranscoded(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("enqueued = %d, want 1", n)
	}
}

func TestBulkEnqueueFailed(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	mFailed := newMovieItem(lib.ID, "F", "/l/failed.mkv")
	mDone := newMovieItem(lib.ID, "D", "/l/done.mkv")
	for _, m := range []*models.MediaItem{mFailed, mDone} {
		if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
			t.Fatal(err)
		}
	}

	jFailed := &models.TranscodeJob{MediaItemID: mFailed.ID}
	jDone := &models.TranscodeJob{MediaItemID: mDone.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, jFailed); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobStatus(ctx, db, jFailed.ID, models.TranscodeStatusFailed, nil); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.CreateTranscodeJob(ctx, db, jDone); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobStatus(ctx, db, jDone.ID, models.TranscodeStatusDone, nil); err != nil {
		t.Fatal(err)
	}

	n, err := sqlite.BulkEnqueueFailed(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("enqueued = %d, want 1", n)
	}
}

func TestGetTranscodeJobByMediaItem(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "Film", "/l/film.mkv")
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	// Not found initially
	_, err := sqlite.GetTranscodeJobByMediaItem(ctx, db, m.ID)
	if err == nil {
		t.Fatal("expected error when no job exists")
	}

	j := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, j); err != nil {
		t.Fatal(err)
	}

	got, err := sqlite.GetTranscodeJobByMediaItem(ctx, db, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != j.ID {
		t.Errorf("got job ID %v, want %v", got.ID, j.ID)
	}
}

func TestResetTranscodeJob(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "Film", "/l/film.mkv")
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	j := &models.TranscodeJob{MediaItemID: m.ID, Priority: 5}
	if err := sqlite.CreateTranscodeJob(ctx, db, j); err != nil {
		t.Fatal(err)
	}

	// Update priority in DB manually to check if preserved
	_, err := db.ExecContext(ctx, `UPDATE transcode_jobs SET priority = 5 WHERE id = ?`, j.ID.String())
	if err != nil {
		t.Fatal(err)
	}

	// Reset job
	if err := sqlite.ResetTranscodeJob(ctx, db, j); err != nil {
		t.Fatal(err)
	}

	got, err := sqlite.GetTranscodeJobByID(ctx, db, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.TranscodeStatusPending {
		t.Errorf("status = %q, want pending", got.Status)
	}
	if got.Priority != 5 {
		t.Errorf("priority = %d, want 5", got.Priority)
	}
	if got.Progress != 0 {
		t.Errorf("progress = %f, want 0", got.Progress)
	}
}

func TestUniqueTranscodeJobConstraint(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := newMovieItem(lib.ID, "Film", "/l/film.mkv")
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	j1 := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, j1); err != nil {
		t.Fatal(err)
	}

	j2 := &models.TranscodeJob{MediaItemID: m.ID}
	err := sqlite.CreateTranscodeJob(ctx, db, j2)
	if err == nil {
		t.Fatal("expected UNIQUE constraint failure error when creating second job for same media item")
	}
}


func TestBulkEnqueueCompleted(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	mDone := newMovieItem(lib.ID, "D", "/l/done.mkv")
	mFailed := newMovieItem(lib.ID, "F", "/l/failed.mkv")
	for _, m := range []*models.MediaItem{mDone, mFailed} {
		if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
			t.Fatal(err)
		}
	}

	jDone := &models.TranscodeJob{MediaItemID: mDone.ID}
	jFailed := &models.TranscodeJob{MediaItemID: mFailed.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, jDone); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobStatus(ctx, db, jDone.ID, models.TranscodeStatusDone, nil); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.CreateTranscodeJob(ctx, db, jFailed); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobStatus(ctx, db, jFailed.ID, models.TranscodeStatusFailed, nil); err != nil {
		t.Fatal(err)
	}

	n, err := sqlite.BulkEnqueueCompleted(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("enqueued = %d, want 1", n)
	}

	// Verify job is pending
	j, err := sqlite.GetTranscodeJobByID(ctx, db, jDone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if j.Status != models.TranscodeStatusPending {
		t.Errorf("status = %s, want pending", j.Status)
	}

	// Verify media_item status is pending
	item, err := sqlite.GetMediaItemByID(ctx, db, mDone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.TranscodeStatus != models.TranscodeStatusPending {
		t.Errorf("media item transcode status = %s, want pending", item.TranscodeStatus)
	}
}

func TestClaimNextSubJob_Pinning(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	m1 := newMovieItem(lib.ID, "A", "/l/a.mkv")
	m2 := newMovieItem(lib.ID, "B", "/l/b.mkv")
	for _, m := range []*models.MediaItem{m1, m2} {
		if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
			t.Fatal(err)
		}
	}

	j1 := &models.TranscodeJob{MediaItemID: m1.ID}
	j2 := &models.TranscodeJob{MediaItemID: m2.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, j1); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.CreateTranscodeJob(ctx, db, j2); err != nil {
		t.Fatal(err)
	}

	// Create two workers
	w1, err := sqlite.CreateWorker(ctx, db, "Worker1")
	if err != nil {
		t.Fatal(err)
	}
	w2, err := sqlite.CreateWorker(ctx, db, "Worker2")
	if err != nil {
		t.Fatal(err)
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, "UPDATE transcode_workers SET last_heartbeat = ?, status = 'idle' WHERE id = ?", nowStr, w1.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE transcode_workers SET last_heartbeat = ?, status = 'idle' WHERE id = ?", nowStr, w2.ID.String()); err != nil {
		t.Fatal(err)
	}

	// --- 1. Basic Remote Worker Pinning ---
	// w1 claims a sub-job from j1 (first job in FIFO order)
	claimed1, err := sqlite.ClaimNextSubJob(ctx, db, &w1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed1 == nil || claimed1.JobID != j1.ID {
		t.Fatalf("expected w1 to claim from j1, got: %+v", claimed1)
	}

	// w2 claims a sub-job. Since j1 is now pinned to active w1, w2 should NOT claim from j1.
	// It should claim from j2 instead.
	claimed2, err := sqlite.ClaimNextSubJob(ctx, db, &w2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed2 == nil || claimed2.JobID != j2.ID {
		t.Fatalf("expected w2 to claim from j2, got: %+v", claimed2)
	}

	// w1 claims again. It should get another sub-job from j1 (prioritizing Category 1).
	claimed3, err := sqlite.ClaimNextSubJob(ctx, db, &w1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed3 == nil || claimed3.JobID != j1.ID {
		t.Fatalf("expected w1 to claim subsequent sub-job from j1, got: %+v", claimed3)
	}

	// --- 2. Inactive Worker Resets Pinning ---
	// Mark w1 as inactive (offline)
	if _, err := db.ExecContext(ctx, "UPDATE transcode_workers SET status = 'offline' WHERE id = ?", w1.ID.String()); err != nil {
		t.Fatal(err)
	}

	// Now that w1 is inactive, w2 should be allowed to claim a sub-job of j1 (since pinning is ignored
	// and j1 has priority).
	claimed4, err := sqlite.ClaimNextSubJob(ctx, db, &w2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed4 == nil || claimed4.JobID != j1.ID {
		t.Fatalf("expected w2 to claim from j1 after w1 went inactive, got: %+v", claimed4)
	}
}

func TestClaimNextSubJob_LocalWorkerPinningAndExhaustion(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	m1 := newMovieItem(lib.ID, "A", "/l/a.mkv")
	m2 := newMovieItem(lib.ID, "B", "/l/b.mkv")
	for _, m := range []*models.MediaItem{m1, m2} {
		if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
			t.Fatal(err)
		}
	}

	j1 := &models.TranscodeJob{MediaItemID: m1.ID}
	j2 := &models.TranscodeJob{MediaItemID: m2.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, j1); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.CreateTranscodeJob(ctx, db, j2); err != nil {
		t.Fatal(err)
	}

	w1, err := sqlite.CreateWorker(ctx, db, "Worker1")
	if err != nil {
		t.Fatal(err)
	}
	nowStr := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, "UPDATE transcode_workers SET last_heartbeat = ?, status = 'idle' WHERE id = ?", nowStr, w1.ID.String()); err != nil {
		t.Fatal(err)
	}

	// --- 1. Local Worker Pinning ---
	// Local worker (nil workerID) claims a sub-job. Should get from j1.
	localClaim1, err := sqlite.ClaimNextSubJob(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if localClaim1 == nil || localClaim1.JobID != j1.ID {
		t.Fatalf("expected local worker to claim from j1, got %+v", localClaim1)
	}

	// Remote worker w1 claims a sub-job. Since j1 is pinned to active local worker (nil),
	// w1 must claim from j2.
	remoteClaim1, err := sqlite.ClaimNextSubJob(ctx, db, &w1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if remoteClaim1 == nil || remoteClaim1.JobID != j2.ID {
		t.Fatalf("expected remote worker w1 to claim from j2, got %+v", remoteClaim1)
	}

	// --- 2. Queue Exhaustion (Stealing) ---
	// Complete/claim all remaining sub-jobs of j2 so only j1 (pinned to local worker) has pending sub-jobs.
	// j2 has 5 sub-jobs total (4 video, 1 subtitles).
	// remoteClaim1 was the first. Let's claim the other 4.
	for i := 0; i < 4; i++ {
		c, err := sqlite.ClaimNextSubJob(ctx, db, &w1.ID)
		if err != nil {
			t.Fatal(err)
		}
		if c == nil || c.JobID != j2.ID {
			t.Fatalf("expected subsequent claims to exhaust j2, got %+v on iteration %d", c, i)
		}
	}

	// Verify that j2 has no pending sub-jobs left
	var pendingInJ2 int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM transcode_sub_jobs WHERE job_id = ? AND status = 'pending'", j2.ID.String()).Scan(&pendingInJ2)
	if err != nil {
		t.Fatal(err)
	}
	if pendingInJ2 != 0 {
		t.Fatalf("expected j2 to have 0 pending sub-jobs, got %d", pendingInJ2)
	}

	// Now w1 polls again. There are no other unclaimed jobs, only j1 (which is pinned to local worker).
	// w1 should be allowed to steal a sub-job from j1.
	stealClaim, err := sqlite.ClaimNextSubJob(ctx, db, &w1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stealClaim == nil || stealClaim.JobID != j1.ID {
		t.Fatalf("expected w1 to steal from j1, got %+v", stealClaim)
	}
}

func TestCreateTranscodeJob_CapsBitrateSmart(t *testing.T) {
	db := openTestDB(t)
	
	// Activate H.264 and AV1 profiles to verify relative scaling
	_, err := db.Exec("UPDATE transcode_profiles SET is_active = 1 WHERE name IN ('360p', '360p (AV1)')")
	if err != nil {
		t.Fatal(err)
	}

	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}

	// Case 1: Source is H.264 at 100 kbps (10,000,000 bytes / 800 seconds)
	m1 := newMovieItem(lib.ID, "LowBitrateH264", "/l/low_h264.mkv")
	m1.FileSize = 10000000
	m1.Duration = 800
	m1.VideoCodec = "h264"
	if err := sqlite.UpsertMediaItem(context.Background(), db, m1); err != nil {
		t.Fatal(err)
	}

	j1 := &models.TranscodeJob{MediaItemID: m1.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, j1); err != nil {
		t.Fatalf("CreateTranscodeJob 1: %v", err)
	}

	subJobs1, err := sqlite.ListTranscodeSubJobsByJob(context.Background(), db, j1.ID)
	if err != nil {
		t.Fatalf("ListTranscodeSubJobsByJob 1: %v", err)
	}

	var checkedH264, checkedAV1 bool
	for _, sj := range subJobs1 {
		if sj.Type == string(models.SubJobTypeVideo) {
			if sj.Codec == nil || sj.VideoBitrateK == nil {
				continue
			}
			if *sj.Codec == "h264" {
				checkedH264 = true
				// H.264 -> H.264: cap is 100 kbps
				if *sj.VideoBitrateK != 100 {
					t.Errorf("expected H264 profile target to be capped at 100, got %d", *sj.VideoBitrateK)
				}
			} else if *sj.Codec == "av1" {
				checkedAV1 = true
				// H.264 (1.0) -> AV1 (0.5): cap is 100 * 0.5 / 1.0 = 50 kbps
				if *sj.VideoBitrateK != 50 {
					t.Errorf("expected AV1 profile target to be capped at 50, got %d", *sj.VideoBitrateK)
				}
			}
		}
	}
	if !checkedH264 || !checkedAV1 {
		t.Errorf("expected to check both H.264 and AV1 sub-jobs for Case 1, got checkedH264=%v, checkedAV1=%v", checkedH264, checkedAV1)
	}

	// Case 2: Source is AV1 at 50 kbps (5,000,000 bytes / 800 seconds)
	m2 := newMovieItem(lib.ID, "LowBitrateAV1", "/l/low_av1.mkv")
	m2.FileSize = 5000000
	m2.Duration = 800
	m2.VideoCodec = "av1"
	if err := sqlite.UpsertMediaItem(context.Background(), db, m2); err != nil {
		t.Fatal(err)
	}

	j2 := &models.TranscodeJob{MediaItemID: m2.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, j2); err != nil {
		t.Fatalf("CreateTranscodeJob 2: %v", err)
	}

	subJobs2, err := sqlite.ListTranscodeSubJobsByJob(context.Background(), db, j2.ID)
	if err != nil {
		t.Fatalf("ListTranscodeSubJobsByJob 2: %v", err)
	}

	checkedH264 = false
	checkedAV1 = false
	for _, sj := range subJobs2 {
		if sj.Type == string(models.SubJobTypeVideo) {
			if sj.Codec == nil || sj.VideoBitrateK == nil {
				continue
			}
			if *sj.Codec == "h264" {
				checkedH264 = true
				// AV1 (0.5) -> H.264 (1.0): cap is 50 * 1.0 / 0.5 = 100 kbps
				if *sj.VideoBitrateK != 100 {
					t.Errorf("expected H264 profile target to be capped at 100 for AV1 source, got %d", *sj.VideoBitrateK)
				}
			} else if *sj.Codec == "av1" {
				checkedAV1 = true
				// AV1 (0.5) -> AV1 (0.5): cap is 50 * 0.5 / 0.5 = 50 kbps
				if *sj.VideoBitrateK != 50 {
					t.Errorf("expected AV1 profile target to be capped at 50 for AV1 source, got %d", *sj.VideoBitrateK)
				}
			}
		}
	}
	if !checkedH264 || !checkedAV1 {
		t.Errorf("expected to check both H.264 and AV1 sub-jobs for Case 2, got checkedH264=%v, checkedAV1=%v", checkedH264, checkedAV1)
	}
}

func TestClaimNextSubJob_RespectsShowEpisodeOrder(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := newLib("/l", models.MediaTypeTV)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	// Create TV Show
	show := &models.TVShow{
		ID:        uuid.New(),
		LibraryID: lib.ID,
		Title:     "Test Show",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := sqlite.CreateTVShow(ctx, db, show); err != nil {
		t.Fatal(err)
	}

	// Create TV Seasons
	season1 := &models.TVSeason{
		ID:           uuid.New(),
		TVShowID:     show.ID,
		SeasonNumber: 1,
		Title:        "Season 1",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := sqlite.CreateTVSeason(ctx, db, season1); err != nil {
		t.Fatal(err)
	}
	season2 := &models.TVSeason{
		ID:           uuid.New(),
		TVShowID:     show.ID,
		SeasonNumber: 2,
		Title:        "Season 2",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := sqlite.CreateTVSeason(ctx, db, season2); err != nil {
		t.Fatal(err)
	}

	// Create episodes in reverse order of how they should be processed
	epS2E1 := &models.MediaItem{
		ID:            uuid.New(),
		LibraryID:     lib.ID,
		Title:         "Episode S2E1",
		MediaType:     "episode",
		FilePath:      "/l/s2e1.mkv",
		TVShowID:      &show.ID,
		TVSeasonID:    &season2.ID,
		SeasonNumber:  &season2.SeasonNumber,
		EpisodeNumber: 1,
	}
	epS1E2 := &models.MediaItem{
		ID:            uuid.New(),
		LibraryID:     lib.ID,
		Title:         "Episode S1E2",
		MediaType:     "episode",
		FilePath:      "/l/s1e2.mkv",
		TVShowID:      &show.ID,
		TVSeasonID:    &season1.ID,
		SeasonNumber:  &season1.SeasonNumber,
		EpisodeNumber: 2,
	}
	epS1E1 := &models.MediaItem{
		ID:            uuid.New(),
		LibraryID:     lib.ID,
		Title:         "Episode S1E1",
		MediaType:     "episode",
		FilePath:      "/l/s1e1.mkv",
		TVShowID:      &show.ID,
		TVSeasonID:    &season1.ID,
		SeasonNumber:  &season1.SeasonNumber,
		EpisodeNumber: 1,
	}

	for _, ep := range []*models.MediaItem{epS2E1, epS1E2, epS1E1} {
		if err := sqlite.UpsertMediaItem(ctx, db, ep); err != nil {
			t.Fatal(err)
		}
	}

	// We manually create transcode jobs with identical created_at times to force fallback sorting
	now := time.Now().UTC()
	
	// Create jobs in database
	jS2E1 := &models.TranscodeJob{MediaItemID: epS2E1.ID, CreatedAt: now}
	jS1E2 := &models.TranscodeJob{MediaItemID: epS1E2.ID, CreatedAt: now}
	jS1E1 := &models.TranscodeJob{MediaItemID: epS1E1.ID, CreatedAt: now}

	for _, j := range []*models.TranscodeJob{jS2E1, jS1E2, jS1E1} {
		if err := sqlite.CreateTranscodeJob(ctx, db, j); err != nil {
			t.Fatal(err)
		}
		// Reset created_at to make absolutely sure they are identical in DB
		_, err := db.ExecContext(ctx, "UPDATE transcode_jobs SET created_at = ? WHERE id = ?", now.Format("2006-01-02T15:04:05.000000Z"), j.ID.String())
		if err != nil {
			t.Fatal(err)
		}
	}

	// We expect ClaimNextSubJob to first claim all sub-jobs from S1E1, then S1E2, then S2E1.
	// We track the order of unique jobs as they are claimed.
	var uniqueJobs []uuid.UUID
	for {
		claimed, err := sqlite.ClaimNextSubJob(ctx, db, nil)
		if err != nil {
			t.Fatal(err)
		}
		if claimed == nil {
			break
		}
		if len(uniqueJobs) == 0 || uniqueJobs[len(uniqueJobs)-1] != claimed.JobID {
			uniqueJobs = append(uniqueJobs, claimed.JobID)
		}
		err = sqlite.UpdateSubJobStatus(ctx, db, claimed.ID, models.TranscodeStatusDone, nil)
		if err != nil {
			t.Fatal(err)
		}
	}

	if len(uniqueJobs) != 3 {
		t.Fatalf("expected 3 unique jobs claimed in sequence, got: %v", uniqueJobs)
	}
	if uniqueJobs[0] != jS1E1.ID {
		t.Errorf("first claimed job should be S1E1 (%v), got %v", jS1E1.ID, uniqueJobs[0])
	}
	if uniqueJobs[1] != jS1E2.ID {
		t.Errorf("second claimed job should be S1E2 (%v), got %v", jS1E2.ID, uniqueJobs[1])
	}
	if uniqueJobs[2] != jS2E1.ID {
		t.Errorf("third claimed job should be S2E1 (%v), got %v", jS2E1.ID, uniqueJobs[2])
	}
}

func TestClaimNextSubJob_NoInterleaving(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	lib := newLib("/l", models.MediaTypeMovie)
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}

	m1 := newMovieItem(lib.ID, "A", "/l/a.mkv")
	m2 := newMovieItem(lib.ID, "B", "/l/b.mkv")
	for _, m := range []*models.MediaItem{m1, m2} {
		if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
			t.Fatal(err)
		}
	}

	j1 := &models.TranscodeJob{MediaItemID: m1.ID}
	j2 := &models.TranscodeJob{MediaItemID: m2.ID}
	if err := sqlite.CreateTranscodeJob(ctx, db, j1); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.CreateTranscodeJob(ctx, db, j2); err != nil {
		t.Fatal(err)
	}

	// Create remote worker
	w1, err := sqlite.CreateWorker(ctx, db, "Worker1")
	if err != nil {
		t.Fatal(err)
	}
	nowStr := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, "UPDATE transcode_workers SET last_heartbeat = ?, status = 'idle' WHERE id = ?", nowStr, w1.ID.String()); err != nil {
		t.Fatal(err)
	}

	// Local worker claims a sub-job first. It should claim J1.
	cLocal, err := sqlite.ClaimNextSubJob(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cLocal == nil || cLocal.JobID != j1.ID {
		t.Fatalf("expected local worker to claim from j1, got %+v", cLocal)
	}

	// Now remote worker w1 claims. Since J1 is pinned to local pool (nil),
	// w1 MUST claim from J2 (as J1 is Category 3 / pinned to another worker).
	cRemote, err := sqlite.ClaimNextSubJob(ctx, db, &w1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cRemote == nil || cRemote.JobID != j2.ID {
		t.Fatalf("expected remote worker w1 to claim from j2, got %+v", cRemote)
	}

	// Local worker claims again. It should get another sub-job from J1 (pinned to local).
	cLocal2, err := sqlite.ClaimNextSubJob(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cLocal2 == nil || cLocal2.JobID != j1.ID {
		t.Fatalf("expected local worker to claim subsequent sub-job from j1, got %+v", cLocal2)
	}
}




