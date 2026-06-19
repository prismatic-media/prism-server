package sqlite_test

import (
	"context"
	"errors"
	"testing"

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

func TestClaimNextJob_RespectsPriorityThenFIFO(t *testing.T) {
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

	claimed1, err := sqlite.ClaimNextJob(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if claimed1 == nil || claimed1.ID != j2.ID {
		t.Fatalf("expected first claim to be prioritized job %s, got %+v", j2.ID, claimed1)
	}

	claimed2, err := sqlite.ClaimNextJob(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if claimed2 == nil || claimed2.ID != j1.ID {
		t.Fatalf("expected second claim to be remaining job %s, got %+v", j1.ID, claimed2)
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


