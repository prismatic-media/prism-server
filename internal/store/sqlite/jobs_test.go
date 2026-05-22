package sqlite_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/ringmaster217/galactic-media-server/internal/models"
	"github.com/ringmaster217/galactic-media-server/internal/store/sqlite"
)

// seedItemForJob creates a library + media item and returns the item ID.
func seedItemForJob(t *testing.T, db interface {
	ExecContext(context.Context, string, ...any) (interface{}, error)
}) uuid.UUID {
	t.Helper()
	return uuid.Nil // placeholder — we use the existing openTestDB + helpers
}

func TestCreateTranscodeJob(t *testing.T) {
	db := openTestDB(t)
	lib := newLib("L", "/l", models.MediaTypeMovie)
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
	lib := newLib("L", "/l", models.MediaTypeMovie)
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
	lib := newLib("L", "/l", models.MediaTypeMovie)
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
	lib := newLib("L", "/l", models.MediaTypeMovie)
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
	lib := newLib("L", "/l", models.MediaTypeMovie)
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
	lib := newLib("L", "/l", models.MediaTypeMovie)
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
	lib := newLib("L", "/l", models.MediaTypeMovie)
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
	lib := newLib("L", "/l", models.MediaTypeMovie)
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
