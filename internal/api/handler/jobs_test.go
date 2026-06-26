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

	"github.com/prismatic-media/prism-server/internal/api/handler"
	apimw "github.com/prismatic-media/prism-server/internal/api/middleware"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/internal/transcoder"
	"github.com/prismatic-media/prism-server/pkg/dash"
)


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
		r.Get("/api/v1/movies/{id}", mediaH.GetMedia)
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs", jobsH.CreateJob)
		r.With(apimw.RequireAdmin).Get("/api/v1/jobs", jobsH.ListJobs)
		r.With(apimw.RequireAdmin).Get("/api/v1/jobs/{id}", jobsH.GetJob)
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs/{id}:prioritize", jobsH.PrioritizeJob)
		r.With(apimw.RequireAdmin).Get("/api/v1/jobs/{id}/progress", jobsH.JobProgress)
	})
	t.Cleanup(func() { _ = db.Close() })

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
	_ = json.NewDecoder(loginRec.Body).Decode(&loginResp)
	token := loginResp["access_token"].(string)
	auth := map[string]string{"Authorization": "Bearer " + token}

	rec := do(t, r, http.MethodPost, "/api/v1/jobs", jsonBody(map[string]any{"media_item_id": item.ID.String()}), auth)
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
	t.Cleanup(func() { _ = db.Close() })
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
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs", jobsH.CreateJob)
	})

	do(t, r, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "admin", "email": "a@x.com", "password": "pw"}), nil)
	lr := do(t, r, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "admin", "password": "pw"}), nil)
	var lresp map[string]any
	_ = json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	rec := do(t, r, http.MethodPost, "/api/v1/jobs", jsonBody(map[string]string{"filter": "bad"}), auth)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestBulkEnqueueJobs_Untranscoded(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
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
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs", jobsH.CreateJob)
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
	_ = json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	rec := do(t, r, http.MethodPost, "/api/v1/jobs", jsonBody(map[string]string{"filter": "untranscoded"}), auth)
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
	t.Cleanup(func() { _ = db.Close() })
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
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs", jobsH.CreateJob)
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
	_ = json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	rec := do(t, r, http.MethodPost, "/api/v1/jobs", jsonBody(map[string]string{"filter": "failed"}), auth)
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
	t.Cleanup(func() { _ = db.Close() })
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
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs/{id}:prioritize", jobsH.PrioritizeJob)
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
	_ = json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	// Pending job can be prioritized.
	okRec := do(t, r, http.MethodPost, "/api/v1/jobs/"+pending.ID.String()+":prioritize", nil, auth)
	if okRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", okRec.Code)
	}

	// Non-pending job gets conflict.
	conflictRec := do(t, r, http.MethodPost, "/api/v1/jobs/"+done.ID.String()+":prioritize", nil, auth)
	if conflictRec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", conflictRec.Code)
	}

	// Unknown job id gets not found.
	notFoundRec := do(t, r, http.MethodPost, "/api/v1/jobs/00000000-0000-0000-0000-000000000001:prioritize", nil, auth)
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", notFoundRec.Code)
	}
}

func TestEnqueueTranscode_MediaNotFound(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
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
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs", jobsH.CreateJob)
	})

	do(t, r, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "admin", "email": "a@x.com", "password": "pw"}), nil)
	lr := do(t, r, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "admin", "password": "pw"}), nil)
	var lresp map[string]any
	_ = json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	rec := do(t, r, http.MethodPost, "/api/v1/jobs", jsonBody(map[string]any{"media_item_id": "00000000-0000-0000-0000-000000000001"}), auth)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestListJobs_Empty(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
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
	_ = json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	rec := do(t, r, http.MethodGet, "/api/v1/jobs", nil, auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var jobs []any
	_ = json.NewDecoder(rec.Body).Decode(&jobs)
	if len(jobs) != 0 {
		t.Errorf("want empty array, got %v", jobs)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
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
	_ = json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	rec := do(t, r, http.MethodGet, "/api/v1/jobs/00000000-0000-0000-0000-000000000001", nil, auth)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestJobProgressWS_TerminalJob(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

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
		r.With(apimw.RequireAdmin).Get("/api/v1/jobs/{id}/progress", jobsH.JobProgress)
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
	_ = json.NewDecoder(lr.Body).Decode(&lresp)
	token := lresp["access_token"].(string)

	// Spin up a test HTTP server with the chi router.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Inject the Authorization header into the request context.
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)
	}))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:] + "/api/v1/jobs/" + job.ID.String() + "/progress"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Authorization": {"Bearer " + token},
	})
	if err != nil {
		t.Fatalf("dial WS: %v", err)
	}
	defer func() { _ = conn.Close() }()

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

func TestBulkEnqueueJobs_CompletedFilter(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
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
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs", jobsH.CreateJob)
	})

	lib := &models.Library{Path: "/lc", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	mDone := &models.MediaItem{LibraryID: lib.ID, Title: "A", MediaType: models.MediaTypeMovie, FilePath: "/lc/a.mkv", TranscodeStatus: models.TranscodeStatusPending}
	if err := sqlite.UpsertMediaItem(context.Background(), db, mDone); err != nil {
		t.Fatal(err)
	}
	jDone := &models.TranscodeJob{MediaItemID: mDone.ID}
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
	_ = json.NewDecoder(lr.Body).Decode(&lresp)
	auth := map[string]string{"Authorization": "Bearer " + lresp["access_token"].(string)}

	rec := do(t, r, http.MethodPost, "/api/v1/jobs", jsonBody(map[string]string{"filter": "completed"}), auth)
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

func TestEnqueueTranscode_Force(t *testing.T) {
	db := openTestDB(t)
	mpdCache := &dash.Cache{}
	pool := transcoder.NewPool(db, 0, mpdCache, nil)

	authH := handler.NewAuthHandler(db, testSecret)
	userH := handler.NewUsersHandler(db, testSecret)
	jobsH := handler.NewJobsHandler(db, pool)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Post("/api/v1/auth/login", authH.Login)
	r.With(apimw.OptionalAuthenticate(testSecret)).Post("/api/v1/users", userH.CreateUser)
	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.With(apimw.RequireAdmin).Post("/api/v1/jobs", jobsH.CreateJob)
	})
	t.Cleanup(func() { _ = db.Close() })

	lib := &models.Library{Path: "/l", MediaType: models.MediaTypeMovie}
	if err := sqlite.CreateLibrary(context.Background(), db, lib); err != nil {
		t.Fatal(err)
	}
	item := &models.MediaItem{
		LibraryID: lib.ID, Title: "Film", MediaType: models.MediaTypeMovie,
		FilePath: "/l/film.mkv", TranscodeStatus: models.TranscodeStatusDone,
		BundleStatus: models.BundleStatusAvailable,
	}
	if err := sqlite.UpsertMediaItem(context.Background(), db, item); err != nil {
		t.Fatal(err)
	}
	// Seed job
	j := &models.TranscodeJob{MediaItemID: item.ID}
	if err := sqlite.CreateTranscodeJob(context.Background(), db, j); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.UpdateJobStatus(context.Background(), db, j.ID, models.TranscodeStatusDone, nil); err != nil {
		t.Fatal(err)
	}

	// Create admin user + log in.
	do(t, r, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "admin", "email": "a@x.com", "password": "pw"}), nil)
	loginRec := do(t, r, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "admin", "password": "pw"}), nil)
	var loginResp map[string]any
	_ = json.NewDecoder(loginRec.Body).Decode(&loginResp)
	token := loginResp["access_token"].(string)
	auth := map[string]string{"Authorization": "Bearer " + token}

	// Request without force should fail/be rejected (since it is already transcoded)
	rec := do(t, r, http.MethodPost, "/api/v1/jobs", jsonBody(map[string]any{"media_item_id": item.ID.String()}), auth)
	if rec.Code == http.StatusAccepted {
		t.Fatalf("expected failure for already transcoded item, got 202")
	}

	// Request with force=true should succeed
	recForce := do(t, r, http.MethodPost, "/api/v1/jobs", jsonBody(map[string]any{"media_item_id": item.ID.String(), "force": true}), auth)
	if recForce.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 for force transcode; body = %s", recForce.Code, recForce.Body)
	}
}

