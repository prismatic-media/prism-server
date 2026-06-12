package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"

	"github.com/ringmaster217/prism/internal/api/handler"
	apimw "github.com/ringmaster217/prism/internal/api/middleware"
	"github.com/ringmaster217/prism/internal/models"
	"github.com/ringmaster217/prism/internal/store/sqlite"
	"github.com/ringmaster217/prism/internal/transcoder"
	"github.com/ringmaster217/prism/pkg/dash"
)

// newJobsRouter builds a minimal test router with Phase 4 job endpoints.
// Returns the router, the transcoder.Pool (for direct manipulation), and a cleanup func.
func newJobsRouter(t *testing.T) (http.Handler, *transcoder.Pool, func()) {
	t.Helper()
	db := openTestDB(t)

	mpdCache := &dash.Cache{}
	// workers=0 means we don't start any workers — we test the API layer only.
	pool := transcoder.NewPool(db, 0, mpdCache, nil)

	authH := handler.NewAuthHandler(db, testSecret)
	userH := handler.NewUsersHandler(db, testSecret)
	jobsH := handler.NewJobsHandler(db, pool)
	mediaH := handler.NewMediaHandler(db)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)

	r.Post("/api/v1/auth/login", authH.Login)
	r.With(apimw.OptionalAuthenticate(testSecret)).Post("/api/v1/users", userH.CreateUser)

	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.Get("/api/v1/media/{id}", mediaH.GetMedia)
		r.With(apimw.RequireAdmin).Post("/api/v1/media/{id}/transcode", jobsH.EnqueueTranscode)
		r.With(apimw.RequireAdmin).Get("/api/v1/jobs", jobsH.ListJobs)
		r.With(apimw.RequireAdmin).Get("/api/v1/jobs/{id}", jobsH.GetJob)
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs/bulk-enqueue", jobsH.BulkEnqueueJobs)
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs/{id}/prioritize", jobsH.PrioritizeJob)
		r.With(apimw.RequireAdmin).Get("/api/v1/ws/jobs/{id}", jobsH.JobProgress)
	})

	cleanup := func() { db.Close() }
	return r, pool, cleanup
}

// seedMediaItem inserts a library + media item in the jobs test DB.
func seedMediaItemForJobTest(t *testing.T, r http.Handler, adminToken string) string {
	t.Helper()
	// We need direct DB access; grab it from the router's underlying store
	// via a helper that creates the item directly in the test DB.
	// Since the router and db share context, just call the REST create-library
	// route isn't wired here — create item via the sqlite store directly.
	// Use a raw SQL approach: the test DB is accessible from the parent test.
	return "" // caller builds the item directly using seedJobItem instead
}

// seedJobItem creates a library + item in db and returns the item.
func seedJobItem(t *testing.T) (interface{ Close() error }, *models.MediaItem) {
	t.Helper()
	// This helper is used in the standalone test that has its own DB.
	return nil, nil
}

func TestEnqueueTranscode_Success(t *testing.T) {
	db := openTestDB(t)
	mpdCache := &dash.Cache{}
	pool := transcoder.NewPool(db, 0, mpdCache, nil)

	authH := handler.NewAuthHandler(db, testSecret)
	userH := handler.NewUsersHandler(db, testSecret)
	jobsH := handler.NewJobsHandler(db, pool)
	mediaH := handler.NewMediaHandler(db)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Post("/api/v1/auth/login", authH.Login)
	r.With(apimw.OptionalAuthenticate(testSecret)).Post("/api/v1/users", userH.CreateUser)
	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.Get("/api/v1/media/{id}", mediaH.GetMedia)
		r.With(apimw.RequireAdmin).Post("/api/v1/media/{id}/transcode", jobsH.EnqueueTranscode)
		r.With(apimw.RequireAdmin).Get("/api/v1/jobs", jobsH.ListJobs)
		r.With(apimw.RequireAdmin).Get("/api/v1/jobs/{id}", jobsH.GetJob)
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs/bulk-enqueue", jobsH.BulkEnqueueJobs)
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs/{id}/prioritize", jobsH.PrioritizeJob)
		r.With(apimw.RequireAdmin).Get("/api/v1/ws/jobs/{id}", jobsH.JobProgress)
	})
	t.Cleanup(func() { db.Close() })

	// Seed a library + media item.
	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	item := &models.MediaItem{
		LibraryID: lib.ID, Title: "Film", MediaType: models.MediaTypeMovie,
		FilePath: "/l/film.mkv", TranscodeStatus: models.TranscodeStatusPending,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, item); err != nil {
		t.Fatal(err)
	}

	// Create admin user + log in.
	do(t, r, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "admin", "email": "a@x.com", "password": "pw"}), nil)
	loginRec := do(t, r, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "admin", "password": "pw"}), nil)
	var loginResp map[string]any
	json.NewDecoder(loginRec.Body).Decode(&loginResp)
	token := loginResp["access_token"].(string)
	auth := map[string]string{"Authorization": "Bearer " + token}

	rec := do(t, r, http.MethodPost, "/api/v1/media/"+item.ID.String()+"/transcode", nil, auth)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body = %s", rec.Code, rec.Body)
	}

	var job models.TranscodeJob
	if err := json.NewDecoder(rec.Body).Decode(&job); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if job.MediaItemID != item.ID {
		t.Errorf("MediaItemID mismatch")
	}
	if job.Status != models.TranscodeStatusPending {
		t.Errorf("status = %q, want pending", job.Status)
	}
}

func TestBulkEnqueueJobs_UnknownFilter(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })
	pool := transcoder.NewPool(db, 0, &dash.Cache{}, nil)
	jobsH := handler.NewJobsHandler(db, pool)
	authH := handler.NewAuthHandler(db, testSecret)
	userH := handler.NewUsersHandler(db, testSecret)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Post("/api/v1/auth/login", authH.Login)
	r.With(apimw.OptionalAuthenticate(testSecret)).Post("/api/v1/users", userH.CreateUser)
	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs/bulk-enqueue", jobsH.BulkEnqueueJobs)
	})

	do(t, r, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "admin", "email": "a@x.com", "password": "pw"}), nil)
	lr := do(t, r, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "admin", "password": "pw"}), nil)
	var lresp map[string]any
	json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	rec := do(t, r, http.MethodPost, "/api/v1/jobs/bulk-enqueue", jsonBody(map[string]string{"filter": "bad"}), auth)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestBulkEnqueueJobs_Untranscoded(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })
	pool := transcoder.NewPool(db, 0, &dash.Cache{}, nil)
	jobsH := handler.NewJobsHandler(db, pool)
	authH := handler.NewAuthHandler(db, testSecret)
	userH := handler.NewUsersHandler(db, testSecret)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Post("/api/v1/auth/login", authH.Login)
	r.With(apimw.OptionalAuthenticate(testSecret)).Post("/api/v1/users", userH.CreateUser)
	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs/bulk-enqueue", jobsH.BulkEnqueueJobs)
	})

	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m1 := &models.MediaItem{LibraryID: lib.ID, Title: "A", MediaType: models.MediaTypeMovie, FilePath: "/l/a.mkv", TranscodeStatus: models.TranscodeStatusPending}
	m2 := &models.MediaItem{LibraryID: lib.ID, Title: "B", MediaType: models.MediaTypeMovie, FilePath: "/l/b.mkv", TranscodeStatus: models.TranscodeStatusPending}
	if err := sqlite.UpsertMediaItem(context.Background(), db, m1); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, m2); err != nil {
		t.Fatal(err)
	}
	// Seed one existing job so only one media item is untouched.
	j := &models.TranscodeJob{MediaItemID: m1.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, j); err != nil {
		t.Fatal(err)
	}

	do(t, r, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "admin", "email": "a@x.com", "password": "pw"}), nil)
	lr := do(t, r, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "admin", "password": "pw"}), nil)
	var lresp map[string]any
	json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	rec := do(t, r, http.MethodPost, "/api/v1/jobs/bulk-enqueue", jsonBody(map[string]string{"filter": "untranscoded"}), auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]int
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["enqueued"] != 1 {
		t.Fatalf("enqueued = %d, want 1", body["enqueued"])
	}
}

func TestBulkEnqueueJobs_FailedFilter(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })
	pool := transcoder.NewPool(db, 0, &dash.Cache{}, nil)
	jobsH := handler.NewJobsHandler(db, pool)
	authH := handler.NewAuthHandler(db, testSecret)
	userH := handler.NewUsersHandler(db, testSecret)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Post("/api/v1/auth/login", authH.Login)
	r.With(apimw.OptionalAuthenticate(testSecret)).Post("/api/v1/users", userH.CreateUser)
	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs/bulk-enqueue", jobsH.BulkEnqueueJobs)
	})

	lib := &models.Library{Path: "/lf", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	mFailed := &models.MediaItem{LibraryID: lib.ID, Title: "A", MediaType: models.MediaTypeMovie, FilePath: "/lf/a.mkv", TranscodeStatus: models.TranscodeStatusPending}
	mDone := &models.MediaItem{LibraryID: lib.ID, Title: "B", MediaType: models.MediaTypeMovie, FilePath: "/lf/b.mkv", TranscodeStatus: models.TranscodeStatusPending}
	if err := sqlite.UpsertMediaItem(context.Background(), db, mFailed); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, mDone); err != nil {
		t.Fatal(err)
	}
	jFailed := &models.TranscodeJob{MediaItemID: mFailed.ID}
	jDone := &models.TranscodeJob{MediaItemID: mDone.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, jFailed); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobStatus(context.Background(), db, jFailed.ID, models.TranscodeStatusFailed, nil); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, jDone); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobStatus(context.Background(), db, jDone.ID, models.TranscodeStatusDone, nil); err != nil {
		t.Fatal(err)
	}

	do(t, r, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "admin", "email": "a@x.com", "password": "pw"}), nil)
	lr := do(t, r, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "admin", "password": "pw"}), nil)
	var lresp map[string]any
	json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	rec := do(t, r, http.MethodPost, "/api/v1/jobs/bulk-enqueue", jsonBody(map[string]string{"filter": "failed"}), auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]int
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["enqueued"] != 1 {
		t.Fatalf("enqueued = %d, want 1", body["enqueued"])
	}
}

func TestPrioritizeJob_StatusCodes(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })
	pool := transcoder.NewPool(db, 0, &dash.Cache{}, nil)
	jobsH := handler.NewJobsHandler(db, pool)
	authH := handler.NewAuthHandler(db, testSecret)
	userH := handler.NewUsersHandler(db, testSecret)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Post("/api/v1/auth/login", authH.Login)
	r.With(apimw.OptionalAuthenticate(testSecret)).Post("/api/v1/users", userH.CreateUser)
	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs/{id}/prioritize", jobsH.PrioritizeJob)
	})

	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	m1 := &models.MediaItem{LibraryID: lib.ID, Title: "A", MediaType: models.MediaTypeMovie, FilePath: "/l/a.mkv", TranscodeStatus: models.TranscodeStatusPending}
	if err := sqlite.UpsertMediaItem(context.Background(), db, m1); err != nil {
		t.Fatal(err)
	}
	m2 := &models.MediaItem{LibraryID: lib.ID, Title: "B", MediaType: models.MediaTypeMovie, FilePath: "/l/b.mkv", TranscodeStatus: models.TranscodeStatusPending}
	if err := sqlite.UpsertMediaItem(context.Background(), db, m2); err != nil {
		t.Fatal(err)
	}
	pending := &models.TranscodeJob{MediaItemID: m1.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, pending); err != nil {
		t.Fatal(err)
	}
	done := &models.TranscodeJob{MediaItemID: m2.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, done); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobStatus(context.Background(), db, done.ID, models.TranscodeStatusDone, nil); err != nil {
		t.Fatal(err)
	}

	do(t, r, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "admin", "email": "a@x.com", "password": "pw"}), nil)
	lr := do(t, r, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "admin", "password": "pw"}), nil)
	var lresp map[string]any
	json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	// Pending job can be prioritized.
	okRec := do(t, r, http.MethodPost, "/api/v1/jobs/"+pending.ID.String()+"/prioritize", nil, auth)
	if okRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", okRec.Code)
	}

	// Non-pending job gets conflict.
	conflictRec := do(t, r, http.MethodPost, "/api/v1/jobs/"+done.ID.String()+"/prioritize", nil, auth)
	if conflictRec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", conflictRec.Code)
	}

	// Unknown job id gets not found.
	notFoundRec := do(t, r, http.MethodPost, "/api/v1/jobs/00000000-0000-0000-0000-000000000001/prioritize", nil, auth)
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", notFoundRec.Code)
	}
}

func TestEnqueueTranscode_MediaNotFound(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })
	mpdCache := &dash.Cache{}
	pool := transcoder.NewPool(db, 0, mpdCache, nil)
	jobsH := handler.NewJobsHandler(db, pool)
	authH := handler.NewAuthHandler(db, testSecret)
	userH := handler.NewUsersHandler(db, testSecret)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Post("/api/v1/auth/login", authH.Login)
	r.With(apimw.OptionalAuthenticate(testSecret)).Post("/api/v1/users", userH.CreateUser)
	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.With(apimw.RequireAdmin).Post("/api/v1/media/{id}/transcode", jobsH.EnqueueTranscode)
	})

	do(t, r, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "admin", "email": "a@x.com", "password": "pw"}), nil)
	lr := do(t, r, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "admin", "password": "pw"}), nil)
	var lresp map[string]any
	json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	rec := do(t, r, http.MethodPost, "/api/v1/media/00000000-0000-0000-0000-000000000001/transcode", nil, auth)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestListJobs_Empty(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })
	pool := transcoder.NewPool(db, 0, &dash.Cache{}, nil)
	jobsH := handler.NewJobsHandler(db, pool)
	authH := handler.NewAuthHandler(db, testSecret)
	userH := handler.NewUsersHandler(db, testSecret)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Post("/api/v1/auth/login", authH.Login)
	r.With(apimw.OptionalAuthenticate(testSecret)).Post("/api/v1/users", userH.CreateUser)
	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.With(apimw.RequireAdmin).Get("/api/v1/jobs", jobsH.ListJobs)
	})

	do(t, r, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "admin", "email": "a@x.com", "password": "pw"}), nil)
	lr := do(t, r, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "admin", "password": "pw"}), nil)
	var lresp map[string]any
	json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	rec := do(t, r, http.MethodGet, "/api/v1/jobs", nil, auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var jobs []any
	json.NewDecoder(rec.Body).Decode(&jobs)
	if len(jobs) != 0 {
		t.Errorf("want empty array, got %v", jobs)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })
	pool := transcoder.NewPool(db, 0, &dash.Cache{}, nil)
	jobsH := handler.NewJobsHandler(db, pool)
	authH := handler.NewAuthHandler(db, testSecret)
	userH := handler.NewUsersHandler(db, testSecret)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Post("/api/v1/auth/login", authH.Login)
	r.With(apimw.OptionalAuthenticate(testSecret)).Post("/api/v1/users", userH.CreateUser)
	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.With(apimw.RequireAdmin).Get("/api/v1/jobs/{id}", jobsH.GetJob)
	})

	do(t, r, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "admin", "email": "a@x.com", "password": "pw"}), nil)
	lr := do(t, r, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "admin", "password": "pw"}), nil)
	var lresp map[string]any
	json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	rec := do(t, r, http.MethodGet, "/api/v1/jobs/00000000-0000-0000-0000-000000000001", nil, auth)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestJobProgressWS_TerminalJob(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })

	mpdCache := &dash.Cache{}
	pool := transcoder.NewPool(db, 0, mpdCache, nil)
	jobsH := handler.NewJobsHandler(db, pool)
	authH := handler.NewAuthHandler(db, testSecret)
	userH := handler.NewUsersHandler(db, testSecret)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Post("/api/v1/auth/login", authH.Login)
	r.With(apimw.OptionalAuthenticate(testSecret)).Post("/api/v1/users", userH.CreateUser)
	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.With(apimw.RequireAdmin).Get("/api/v1/ws/jobs/{id}", jobsH.JobProgress)
	})

	// Seed a done job.
	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	item := &models.MediaItem{
		LibraryID: lib.ID, Title: "Film", MediaType: models.MediaTypeMovie,
		FilePath: "/l/f.mkv", TranscodeStatus: models.TranscodeStatusDone,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, item); err != nil {
		t.Fatal(err)
	}
	job := &models.TranscodeJob{MediaItemID: item.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, job); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobStatus(context.Background(), db, job.ID, models.TranscodeStatusDone, nil); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobProgress(context.Background(), db, job.ID, 100); err != nil {
		t.Fatal(err)
	}

	// Create admin user + get token.
	do(t, r, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "admin", "email": "a@x.com", "password": "pw"}), nil)
	lr := do(t, r, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "admin", "password": "pw"}), nil)
	var lresp map[string]any
	json.NewDecoder(lr.Body).Decode(&lresp)
	token := lresp["access_token"].(string)

	// Spin up a test HTTP server with the chi router.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Inject the Authorization header into the request context.
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)
	}))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:] + "/api/v1/ws/jobs/" + job.ID.String()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Authorization": {"Bearer " + token},
	})
	if err != nil {
		t.Fatalf("dial WS: %v", err)
	}
	defer conn.Close()

	var evt transcoder.ProgressEvent
	if err := conn.ReadJSON(&evt); err != nil {
		t.Fatalf("read WS message: %v", err)
	}
	if !evt.Done {
		t.Errorf("expected Done=true for completed job")
	}
	if evt.Progress != 100 {
		t.Errorf("progress = %v, want 100", evt.Progress)
	}
}
