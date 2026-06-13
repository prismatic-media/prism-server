package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/prismatic-media/prism-server/internal/api/handler"
	apimw "github.com/prismatic-media/prism-server/internal/api/middleware"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

func setupRouter(db *sql.DB) http.Handler {
	h := handler.NewSetupHandler(db)
	r := chi.NewRouter()
	r.Post("/api/v1/setup", h.CompleteSetup)
	return r
}

func settingsRouter(db *sql.DB) http.Handler {
	h := handler.NewSettingsHandler(db)
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.With(apimw.RequireAdmin).Get("/api/v1/admin/settings", h.GetSettings)
		r.With(apimw.RequireAdmin).Put("/api/v1/admin/settings", h.UpdateSettings)
	})
	return r
}

// bootstrapFresh opens a test DB, runs bootstrap, and resets setup_complete to "false".
func bootstrapFresh(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	if err := sqlite.BootstrapSettings(context.Background(), db); err != nil {
		t.Fatalf("BootstrapSettings: %v", err)
	}
	if err := sqlite.SetSetting(context.Background(), db, "setup_complete", "false"); err != nil {
		t.Fatalf("SetSetting setup_complete=false: %v", err)
	}
	return db
}

// --- Setup handler tests ---

func TestSetup_Success(t *testing.T) {
	db := bootstrapFresh(t)

	body, _ := json.Marshal(map[string]string{
		"username":             "admin",
		"password":             "secret123",
		"segments_dir":         "/custom/segments",
		"thumbs_dir":           "/custom/thumbs",
		"tmdb_api_key":         "tmdb123",
		"cast_receiver_app_id": "app123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	setupRouter(db).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	val, err := sqlite.GetSetting(context.Background(), db, "setup_complete")
	if err != nil || val != "true" {
		t.Errorf("expected setup_complete=true, got %q (err: %v)", val, err)
	}

	tmdbVal, err := sqlite.GetSetting(context.Background(), db, "tmdb_api_key")
	if err != nil || tmdbVal != "tmdb123" {
		t.Errorf("expected tmdb_api_key=tmdb123, got %q (err: %v)", tmdbVal, err)
	}

	castVal, err := sqlite.GetSetting(context.Background(), db, "cast_receiver_app_id")
	if err != nil || castVal != "app123" {
		t.Errorf("expected cast_receiver_app_id=app123, got %q (err: %v)", castVal, err)
	}
}

func TestSetup_AlreadyComplete(t *testing.T) {
	db := openTestDB(t)
	if err := sqlite.BootstrapSettings(context.Background(), db); err != nil {
		t.Fatalf("BootstrapSettings: %v", err)
	}
	if err := sqlite.SetSetting(context.Background(), db, "setup_complete", "true"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"username":     "admin",
		"password":     "secret123",
		"segments_dir": "/custom/segments",
		"thumbs_dir":   "/custom/thumbs",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	setupRouter(db).ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestSetup_MissingFields(t *testing.T) {
	cases := []struct {
		name string
		body map[string]string
	}{
		{"missing password", map[string]string{"username": "admin", "segments_dir": "/data/segments", "thumbs_dir": "/data/thumbs"}},
		{"missing username", map[string]string{"password": "secret", "segments_dir": "/data/segments", "thumbs_dir": "/data/thumbs"}},
		{"missing segments_dir", map[string]string{"username": "admin", "password": "secret", "thumbs_dir": "/data/thumbs"}},
		{"missing thumbs_dir", map[string]string{"username": "admin", "password": "secret", "segments_dir": "/data/segments"}},
		{"all empty", map[string]string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := bootstrapFresh(t)
			b, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			setupRouter(db).ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

// --- Settings handler tests ---

func TestSettings_GetAsAdmin(t *testing.T) {
	db := openTestDB(t)
	if err := sqlite.BootstrapSettings(context.Background(), db); err != nil {
		t.Fatalf("BootstrapSettings: %v", err)
	}
	admin := createUser(t, db, "admin", "admin@test.com", "pass", true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, admin.ID, true))
	w := httptest.NewRecorder()

	settingsRouter(db).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := result["jwt_secret"]; ok {
		t.Error("jwt_secret must not appear in settings response")
	}
	if _, ok := result["setup_complete"]; ok {
		t.Error("setup_complete must not appear in settings response")
	}
	if _, ok := result["transcode_workers"]; !ok {
		t.Error("transcode_workers should be present")
	}
}

func TestSettings_GetForbiddenForNonAdmin(t *testing.T) {
	db := openTestDB(t)
	if err := sqlite.BootstrapSettings(context.Background(), db); err != nil {
		t.Fatalf("BootstrapSettings: %v", err)
	}
	user := createUser(t, db, "user", "user@test.com", "pass", false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, user.ID, false))
	w := httptest.NewRecorder()

	settingsRouter(db).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSettings_Update(t *testing.T) {
	db := openTestDB(t)
	if err := sqlite.BootstrapSettings(context.Background(), db); err != nil {
		t.Fatalf("BootstrapSettings: %v", err)
	}
	admin := createUser(t, db, "admin", "admin@test.com", "pass", true)

	body, _ := json.Marshal(map[string]string{"ffmpeg_hwaccel": "vaapi", "transcode_workers": "4"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, admin.ID, true))
	w := httptest.NewRecorder()

	settingsRouter(db).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	val, _ := sqlite.GetSetting(context.Background(), db, "ffmpeg_hwaccel")
	if val != "vaapi" {
		t.Errorf("ffmpeg_hwaccel not updated, got %q", val)
	}
}

func TestSettings_UnknownKeyRejected(t *testing.T) {
	db := openTestDB(t)
	if err := sqlite.BootstrapSettings(context.Background(), db); err != nil {
		t.Fatalf("BootstrapSettings: %v", err)
	}
	admin := createUser(t, db, "admin", "admin@test.com", "pass", true)

	body, _ := json.Marshal(map[string]string{"unknown_key": "value"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, admin.ID, true))
	w := httptest.NewRecorder()

	settingsRouter(db).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSettings_UnauthenticatedRejected(t *testing.T) {
	db := openTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	w := httptest.NewRecorder()

	settingsRouter(db).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
