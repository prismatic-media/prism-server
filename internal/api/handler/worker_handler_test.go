package handler_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ringmaster217/prism/internal/api/handler"
	"github.com/ringmaster217/prism/internal/models"
	"github.com/ringmaster217/prism/internal/store/sqlite"
	"github.com/ringmaster217/prism/internal/transcoder"
	"github.com/ringmaster217/prism/pkg/dash"
	"github.com/ringmaster217/prism/pkg/events"
)

func TestWorkerAuthMiddleware(t *testing.T) {
	db := openTestDB(t)
	bus := events.NewBus()
	pool := transcoder.NewPool(db, 0, &dash.Cache{}, bus)
	wHandler := handler.NewWorkerHandler(db, pool, bus)

	wModel, err := sqlite.CreateWorker(context.Background(), db, "TestWorker")
	if err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Use(wHandler.Authenticate)
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		worker := handler.WorkerFromContext(r.Context())
		if worker == nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(worker.Name))
	})

	// 1. Missing header
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}

	// 2. Invalid API key
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Worker-API-Key", "invalid-key")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}

	// 3. Valid key
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Worker-API-Key", wModel.APIKey)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "TestWorker" {
		t.Errorf("expected TestWorker, got %q", rec.Body.String())
	}
}

func TestWorkerHeartbeatAndClaim(t *testing.T) {
	db := openTestDB(t)
	bus := events.NewBus()
	pool := transcoder.NewPool(db, 0, &dash.Cache{}, bus)
	wHandler := handler.NewWorkerHandler(db, pool, bus)

	worker, err := sqlite.CreateWorker(context.Background(), db, "WorkerHost")
	if err != nil {
		t.Fatal(err)
	}

	// Create a dummy movie and transcode job
	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m := &models.MediaItem{
		LibraryID:       lib.ID,
		Title:           "Test Movie",
		MediaType:       models.MediaTypeMovie,
		FilePath:        "/l/movie.mkv",
		TranscodeStatus: models.TranscodeStatusNone,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}
	job := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, job); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Use(wHandler.Authenticate)
	r.Post("/heartbeat", wHandler.Heartbeat)

	// Post heartbeat. Worker should claim the pending job.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/heartbeat", nil)
	req.Header.Set("X-Worker-API-Key", worker.APIKey)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Threads int                  `json:"threads"`
		HWAccel string               `json:"hwaccel"`
		Job     *models.TranscodeJob `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if resp.Job == nil {
		t.Fatal("expected worker to claim a job, but got nil")
	}
	if resp.Job.ID != job.ID {
		t.Errorf("claimed job ID mismatch: got %v, want %v", resp.Job.ID, job.ID)
	}

	// Verify worker status is transcoding in DB
	dbWorker, err := sqlite.GetWorkerByID(context.Background(), db, worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dbWorker.Status != "transcoding" {
		t.Errorf("expected worker status transcoding, got %q", dbWorker.Status)
	}

	// Verify job in DB is assigned to worker and processing
	dbJob, err := sqlite.GetTranscodeJobByID(context.Background(), db, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dbJob.Status != models.TranscodeStatusProcessing {
		t.Errorf("expected job status processing, got %q", dbJob.Status)
	}
	if dbJob.WorkerID == nil || *dbJob.WorkerID != worker.ID {
		t.Errorf("expected job assigned to worker %v, got %v", worker.ID, dbJob.WorkerID)
	}
}

func TestWorkerDownloadSource(t *testing.T) {
	db := openTestDB(t)
	wHandler := handler.NewWorkerHandler(db, nil, nil)

	worker, err := sqlite.CreateWorker(context.Background(), db, "WorkerHost")
	if err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "source.mp4")
	if err := os.WriteFile(sourcePath, []byte("movie content"), 0644); err != nil {
		t.Fatal(err)
	}

	lib := &models.Library{Path: tempDir, MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m := &models.MediaItem{
		LibraryID:       lib.ID,
		Title:           "Test Movie",
		MediaType:       models.MediaTypeMovie,
		FilePath:        sourcePath,
		TranscodeStatus: models.TranscodeStatusNone,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Use(wHandler.Authenticate)
	r.Get("/media/{id}/download", wHandler.DownloadSource)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/media/%s/download", m.ID), nil)
	req.Header.Set("X-Worker-API-Key", worker.APIKey)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "movie content" {
		t.Errorf("download content mismatch: got %q", rec.Body.String())
	}
}

func TestWorkerUpdateProgress(t *testing.T) {
	db := openTestDB(t)
	bus := events.NewBus()
	pool := transcoder.NewPool(db, 0, &dash.Cache{}, bus)
	wHandler := handler.NewWorkerHandler(db, pool, bus)

	worker, err := sqlite.CreateWorker(context.Background(), db, "WorkerHost")
	if err != nil {
		t.Fatal(err)
	}

	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m := &models.MediaItem{
		LibraryID:       lib.ID,
		Title:           "Movie",
		MediaType:       models.MediaTypeMovie,
		FilePath:        "/l/movie.mkv",
		TranscodeStatus: models.TranscodeStatusNone,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}
	job := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, job); err != nil {
		t.Fatal(err)
	}
	claimed, err := sqlite.ClaimNextJob(context.Background(), db, &worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	job = claimed

	r := chi.NewRouter()
	r.Use(wHandler.Authenticate)
	r.Post("/jobs/{id}/progress", wHandler.UpdateProgress)

	// 1. Report progress
	body := bytes.NewBufferString(`{"progress": 45.5, "status": "processing"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", fmt.Sprintf("/jobs/%s/progress", job.ID), body)
	req.Header.Set("X-Worker-API-Key", worker.APIKey)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	dbJob, _ := sqlite.GetTranscodeJobByID(context.Background(), db, job.ID)
	if dbJob.Progress != 45.5 {
		t.Errorf("expected progress 45.5, got %f", dbJob.Progress)
	}

	// 2. Report failure
	body = bytes.NewBufferString(`{"progress": 45.5, "status": "failed", "error_msg": "out of disk space"}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", fmt.Sprintf("/jobs/%s/progress", job.ID), body)
	req.Header.Set("X-Worker-API-Key", worker.APIKey)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	dbJob, _ = sqlite.GetTranscodeJobByID(context.Background(), db, job.ID)
	if dbJob.Status != models.TranscodeStatusFailed {
		t.Errorf("expected job status failed, got %q", dbJob.Status)
	}
	if dbJob.ErrorMsg == nil || *dbJob.ErrorMsg != "out of disk space" {
		t.Errorf("expected error message 'out of disk space', got %v", dbJob.ErrorMsg)
	}
}

func TestWorkerUploadBundle(t *testing.T) {
	db := openTestDB(t)
	bus := events.NewBus()
	pool := transcoder.NewPool(db, 0, &dash.Cache{}, bus)
	wHandler := handler.NewWorkerHandler(db, pool, bus)

	worker, err := sqlite.CreateWorker(context.Background(), db, "WorkerHost")
	if err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	// Set up segment storage area
	if err := sqlite.CreateStorageArea(context.Background(), db, &models.StorageArea{
		Kind:    models.StorageAreaKindSegments,
		Path:    tempDir,
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.SetSetting(context.Background(), db, "storage_min_free_bytes", "0"); err != nil {
		t.Fatal(err)
	}

	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	fp := "dummysourcefingerprint"
	m := &models.MediaItem{
		LibraryID:         lib.ID,
		Title:             "Movie",
		MediaType:         models.MediaTypeMovie,
		FilePath:          "/l/movie.mkv",
		SourceFingerprint: &fp,
		TranscodeStatus:   models.TranscodeStatusNone,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, m); err != nil {
		t.Fatal(err)
	}
	job := &models.TranscodeJob{MediaItemID: m.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, job); err != nil {
		t.Fatal(err)
	}
	claimed, err := sqlite.ClaimNextJob(context.Background(), db, &worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	job = claimed

	// Create zip archive containing dummy manifest.mpd
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	f, err := zw.Create("manifest.mpd")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("<MPD></MPD>"))
	zw.Close()

	// Prepare multipart request
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("bundle", "bundle.zip")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(part, &zipBuf)
	writer.Close()

	r := chi.NewRouter()
	r.Use(wHandler.Authenticate)
	r.Post("/jobs/{id}/bundle", wHandler.UploadBundle)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", fmt.Sprintf("/jobs/%s/bundle", job.ID), body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Worker-API-Key", worker.APIKey)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	// Verify segments extracted in target directory
	targetDir := filepath.Join(tempDir, job.MediaItemID.String())
	mpdData, err := os.ReadFile(filepath.Join(targetDir, "manifest.mpd"))
	if err != nil {
		t.Fatal("manifest.mpd was not extracted successfully")
	}
	if string(mpdData) != "<MPD></MPD>" {
		t.Errorf("manifest contents mismatch: %q", string(mpdData))
	}

	// Verify artifact.json is written successfully
	sidecarPath := filepath.Join(targetDir, "artifact.json")
	if _, err := os.Stat(sidecarPath); err != nil {
		t.Errorf("artifact.json sidecar was not created: %v", err)
	}

	// Verify DB state updated
	dbItem, _ := sqlite.GetMediaItemByID(context.Background(), db, job.MediaItemID)
	if dbItem.TranscodeStatus != models.TranscodeStatusDone {
		t.Errorf("expected media transcode status done, got %q", dbItem.TranscodeStatus)
	}
	if dbItem.MPDPath == nil || *dbItem.MPDPath == "" {
		t.Error("expected mpd_path to be set")
	}
	if dbItem.BundleStatus != models.BundleStatusAvailable {
		t.Errorf("expected bundle status available, got %q", dbItem.BundleStatus)
	}

	dbJob, _ := sqlite.GetTranscodeJobByID(context.Background(), db, job.ID)
	if dbJob.Status != models.TranscodeStatusDone {
		t.Errorf("expected job status done, got %q", dbJob.Status)
	}
	if dbJob.Progress != 100 {
		t.Errorf("expected job progress 100, got %f", dbJob.Progress)
	}
}

func TestWorkerAdminCRUD(t *testing.T) {
	db := openTestDB(t)
	admHandler := handler.NewWorkerAdminHandler(db)

	r := chi.NewRouter()
	r.Get("/workers", admHandler.List)
	r.Post("/workers", admHandler.Create)
	r.Put("/workers/{id}", admHandler.Update)
	r.Delete("/workers/{id}", admHandler.Delete)

	// 1. Create worker
	body := bytes.NewBufferString(`{"name": "GamingPC"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/workers", body)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 created, got %d", rec.Code)
	}

	var created models.TranscodeWorker
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Name != "GamingPC" || created.APIKey == "" {
		t.Errorf("created worker invalid properties: %+v", created)
	}

	// 2. List workers
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/workers", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	var list []models.TranscodeWorker
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Errorf("expected list with 1 worker, got %+v", list)
	}

	// 3. Update worker config
	body = bytes.NewBufferString(`{"threads": 4, "hwaccel": "vaapi"}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("PUT", fmt.Sprintf("/workers/%s", created.ID), body)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var updated models.TranscodeWorker
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Threads != 4 || updated.HWAccel != "vaapi" {
		t.Errorf("updated worker settings mismatch: %+v", updated)
	}

	// 4. Delete worker
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", fmt.Sprintf("/workers/%s", created.ID), nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 no content, got %d", rec.Code)
	}

	// List again, should be empty
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/workers", nil)
	r.ServeHTTP(rec, req)
	var emptyList []models.TranscodeWorker
	_ = json.Unmarshal(rec.Body.Bytes(), &emptyList)
	if len(emptyList) != 0 {
		t.Errorf("expected empty list, got %+v", emptyList)
	}
}
