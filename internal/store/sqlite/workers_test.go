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

func TestCreateWorker_SetsProperties(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	w, err := sqlite.CreateWorker(ctx, db, "TestWorker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.ID == uuid.Nil {
		t.Error("expected worker ID to be set")
	}
	if w.Name != "TestWorker" {
		t.Errorf("expected worker Name 'TestWorker', got %q", w.Name)
	}
	if w.APIKey == "" {
		t.Error("expected worker API key to be set")
	}
	if w.Threads != 1 {
		t.Errorf("expected default threads 1, got %d", w.Threads)
	}
	if w.HWAccel != "none" {
		t.Errorf("expected default hwaccel 'none', got %q", w.HWAccel)
	}
	if w.Status != "offline" {
		t.Errorf("expected default status 'offline', got %q", w.Status)
	}
	if w.CreatedAt.IsZero() || w.UpdatedAt.IsZero() {
		t.Error("expected timestamps to be set")
	}
}

func TestGetWorkerByID_FoundAndNotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	w, err := sqlite.CreateWorker(ctx, db, "WorkerA")
	if err != nil {
		t.Fatal(err)
	}

	got, err := sqlite.GetWorkerByID(ctx, db, w.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != w.ID || got.Name != "WorkerA" {
		t.Errorf("retrieved worker mismatch: %+v", got)
	}

	_, err = sqlite.GetWorkerByID(ctx, db, uuid.New())
	if !errors.Is(err, sqlite.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing ID, got %v", err)
	}
}

func TestGetWorkerByAPIKey_FoundAndNotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	w, err := sqlite.CreateWorker(ctx, db, "WorkerB")
	if err != nil {
		t.Fatal(err)
	}

	got, err := sqlite.GetWorkerByAPIKey(ctx, db, w.APIKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != w.ID || got.APIKey != w.APIKey {
		t.Errorf("retrieved worker mismatch: %+v", got)
	}

	_, err = sqlite.GetWorkerByAPIKey(ctx, db, "invalid_key")
	if !errors.Is(err, sqlite.ErrNotFound) {
		t.Errorf("expected ErrNotFound for invalid key, got %v", err)
	}
}

func TestListWorkers(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, _ = sqlite.CreateWorker(ctx, db, "WorkerC")
	_, _ = sqlite.CreateWorker(ctx, db, "WorkerB")
	_, _ = sqlite.CreateWorker(ctx, db, "WorkerA")

	workers, err := sqlite.ListWorkers(ctx, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(workers) != 3 {
		t.Errorf("expected 3 workers, got %d", len(workers))
	}

	// Should be sorted alphabetically by name
	if workers[0].Name != "WorkerA" || workers[1].Name != "WorkerB" || workers[2].Name != "WorkerC" {
		t.Errorf("expected workers sorted alphabetically, got order: %s, %s, %s",
			workers[0].Name, workers[1].Name, workers[2].Name)
	}
}

func TestUpdateWorkerSettings(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	w, err := sqlite.CreateWorker(ctx, db, "WorkerD")
	if err != nil {
		t.Fatal(err)
	}

	err = sqlite.UpdateWorkerSettings(ctx, db, w.ID, 4, "vaapi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := sqlite.GetWorkerByID(ctx, db, w.ID)
	if err != nil {
		t.Fatal(err)
	}

	if got.Threads != 4 || got.HWAccel != "vaapi" {
		t.Errorf("worker settings did not update: threads=%d, hwaccel=%s", got.Threads, got.HWAccel)
	}

	err = sqlite.UpdateWorkerSettings(ctx, db, uuid.New(), 4, "vaapi")
	if !errors.Is(err, sqlite.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing worker, got %v", err)
	}
}

func TestUpdateWorkerHeartbeat(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	w, err := sqlite.CreateWorker(ctx, db, "WorkerE")
	if err != nil {
		t.Fatal(err)
	}

	if w.LastHeartbeat != nil {
		t.Error("expected last heartbeat to be nil initially")
	}

	err = sqlite.UpdateWorkerHeartbeat(ctx, db, w.ID, "idle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := sqlite.GetWorkerByID(ctx, db, w.ID)
	if err != nil {
		t.Fatal(err)
	}

	if got.Status != "idle" {
		t.Errorf("expected status 'idle', got %q", got.Status)
	}
	if got.LastHeartbeat == nil {
		t.Error("expected last heartbeat to be updated")
	} else if time.Since(*got.LastHeartbeat) > 5*time.Second {
		t.Errorf("last heartbeat is too old: %v", got.LastHeartbeat)
	}
}

func TestDeleteWorker(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	w, err := sqlite.CreateWorker(ctx, db, "WorkerF")
	if err != nil {
		t.Fatal(err)
	}

	err = sqlite.DeleteWorker(ctx, db, w.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = sqlite.GetWorkerByID(ctx, db, w.ID)
	if !errors.Is(err, sqlite.ErrNotFound) {
		t.Errorf("expected worker to be deleted, but got err = %v", err)
	}

	err = sqlite.DeleteWorker(ctx, db, uuid.New())
	if !errors.Is(err, sqlite.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing worker, got %v", err)
	}
}

func TestRecoverFailedWorkers(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// 1. Create three workers
	w1, err := sqlite.CreateWorker(ctx, db, "Worker1") // Will fail (status=idle, old heartbeat)
	if err != nil {
		t.Fatal(err)
	}
	w2, err := sqlite.CreateWorker(ctx, db, "Worker2") // Will not fail (status=idle, recent heartbeat)
	if err != nil {
		t.Fatal(err)
	}
	w3, err := sqlite.CreateWorker(ctx, db, "Worker3") // Already offline
	if err != nil {
		t.Fatal(err)
	}

	// Update w1's heartbeat to 1 minute ago
	pastTime := time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, "UPDATE transcode_workers SET last_heartbeat = ?, status = 'idle' WHERE id = ?", pastTime, w1.ID.String()); err != nil {
		t.Fatal(err)
	}

	// Update w2's heartbeat to now
	recentTime := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, "UPDATE transcode_workers SET last_heartbeat = ?, status = 'idle' WHERE id = ?", recentTime, w2.ID.String()); err != nil {
		t.Fatal(err)
	}

	// 2. Set up media library, media item, and job for w1
	lib := &models.Library{Path: "/l_test", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(ctx, db, lib); err != nil {
		t.Fatal(err)
	}
	m := &models.MediaItem{
		LibraryID:       lib.ID,
		Title:           "TestMovie",
		MediaType:       models.MediaTypeMovie,
		FilePath:        "/l_test/movie.mkv",
		TranscodeStatus: models.TranscodeStatusProcessing,
	}
	if err := sqlite.UpsertMediaItem(ctx, db, m); err != nil {
		t.Fatal(err)
	}

	j := &models.TranscodeJob{
		MediaItemID: m.ID,
	}
	if err := sqlite.CreateTranscodeJob(ctx, db, j); err != nil {
		t.Fatal(err)
	}

	// Update job to be processing by w1
	if _, err := db.ExecContext(ctx, "UPDATE transcode_jobs SET status = 'processing', worker_id = ?, progress = 50.0 WHERE id = ?", w1.ID.String(), j.ID.String()); err != nil {
		t.Fatal(err)
	}

	// Update sub-jobs to be processing by w1
	if _, err := db.ExecContext(ctx, "UPDATE transcode_sub_jobs SET status = 'processing', worker_id = ?, progress = 50.0 WHERE job_id = ?", w1.ID.String(), j.ID.String()); err != nil {
		t.Fatal(err)
	}

	// 3. Run RecoverFailedWorkers
	requeued, err := sqlite.RecoverFailedWorkers(ctx, db, 30*time.Second)
	if err != nil {
		t.Fatalf("RecoverFailedWorkers failed: %v", err)
	}

	// 4. Assert w1 is offline and job is re-queued
	w1Got, _ := sqlite.GetWorkerByID(ctx, db, w1.ID)
	if w1Got.Status != "offline" {
		t.Errorf("expected w1 status 'offline', got %q", w1Got.Status)
	}

	w2Got, _ := sqlite.GetWorkerByID(ctx, db, w2.ID)
	if w2Got.Status != "idle" {
		t.Errorf("expected w2 status 'idle', got %q", w2Got.Status)
	}

	w3Got, _ := sqlite.GetWorkerByID(ctx, db, w3.ID)
	if w3Got.Status != "offline" {
		t.Errorf("expected w3 status 'offline', got %q", w3Got.Status)
	}

	// Assert job state
	jGot, _ := sqlite.GetTranscodeJobByID(ctx, db, j.ID)
	if jGot.Status != models.TranscodeStatusPending {
		t.Errorf("expected job status 'pending', got %q", jGot.Status)
	}
	if jGot.WorkerID != nil {
		t.Errorf("expected job worker ID to be nil, got %v", jGot.WorkerID)
	}
	if jGot.Progress != 0 {
		t.Errorf("expected job progress to be 0, got %f", jGot.Progress)
	}

	// Assert media item state
	mGot, _ := sqlite.GetMediaItemByID(ctx, db, m.ID)
	if mGot.TranscodeStatus != models.TranscodeStatusPending {
		t.Errorf("expected media status 'pending', got %q", mGot.TranscodeStatus)
	}

	// Assert return value
	if len(requeued) != 1 {
		t.Fatalf("expected 1 requeued job, got %d", len(requeued))
	}
	if requeued[0].JobID != j.ID || requeued[0].MediaItemID != m.ID || requeued[0].LibraryID != lib.ID {
		t.Errorf("requeued job info mismatch: %+v", requeued[0])
	}
}

func TestEphemeralWorkersAndTokens(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Test Token creation and retrieval
	token, err := sqlite.CreateEphemeralWorkerToken(ctx, db, "TestToken")
	if err != nil {
		t.Fatalf("unexpected error creating token: %v", err)
	}
	if token.Name != "TestToken" {
		t.Errorf("expected token name 'TestToken', got %q", token.Name)
	}
	if token.Token == "" {
		t.Error("expected token string to be non-empty")
	}

	gotToken, err := sqlite.GetEphemeralWorkerTokenByValue(ctx, db, token.Token)
	if err != nil {
		t.Fatalf("unexpected error getting token: %v", err)
	}
	if gotToken.ID != token.ID || gotToken.Name != "TestToken" {
		t.Errorf("token mismatch: %+v", gotToken)
	}

	// Test List tokens
	tokens, err := sqlite.ListEphemeralWorkerTokens(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 || tokens[0].ID != token.ID {
		t.Errorf("expected 1 token, got %d", len(tokens))
	}

	// Test Create Ephemeral Worker
	w, err := sqlite.CreateEphemeralWorker(ctx, db, "ephemeral-1")
	if err != nil {
		t.Fatalf("unexpected error creating worker: %v", err)
	}
	if !w.IsEphemeral {
		t.Error("expected IsEphemeral to be true")
	}
	if w.Status != "idle" {
		t.Errorf("expected status 'idle', got %q", w.Status)
	}

	// Test Get Worker By Name
	gotWorker, err := sqlite.GetWorkerByName(ctx, db, "ephemeral-1")
	if err != nil {
		t.Fatalf("unexpected error getting worker: %v", err)
	}
	if gotWorker.ID != w.ID || !gotWorker.IsEphemeral {
		t.Errorf("worker mismatch: %+v", gotWorker)
	}

	// Test Delete Token
	err = sqlite.DeleteEphemeralWorkerToken(ctx, db, token.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlite.GetEphemeralWorkerTokenByValue(ctx, db, token.Token)
	if !errors.Is(err, sqlite.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestRecoverFailedWorkers_DeletesEphemeral(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Create an ephemeral worker and standard worker
	ew, err := sqlite.CreateEphemeralWorker(ctx, db, "ephemeral-failed")
	if err != nil {
		t.Fatal(err)
	}
	sw, err := sqlite.CreateWorker(ctx, db, "standard-failed")
	if err != nil {
		t.Fatal(err)
	}

	// Set their heartbeats to 1 minute ago
	pastTime := time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, "UPDATE transcode_workers SET last_heartbeat = ?, status = 'idle' WHERE id = ?", pastTime, ew.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE transcode_workers SET last_heartbeat = ?, status = 'idle' WHERE id = ?", pastTime, sw.ID.String()); err != nil {
		t.Fatal(err)
	}

	// Run RecoverFailedWorkers
	_, err = sqlite.RecoverFailedWorkers(ctx, db, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// Standard worker should be offline
	swGot, err := sqlite.GetWorkerByID(ctx, db, sw.ID)
	if err != nil {
		t.Fatal(err)
	}
	if swGot.Status != "offline" {
		t.Errorf("expected standard worker status 'offline', got %q", swGot.Status)
	}

	// Ephemeral worker should be completely deleted
	_, err = sqlite.GetWorkerByID(ctx, db, ew.ID)
	if !errors.Is(err, sqlite.ErrNotFound) {
		t.Errorf("expected ephemeral worker to be deleted, got err = %v", err)
	}
}

