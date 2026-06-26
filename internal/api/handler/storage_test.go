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
	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/api/handler"
	apimw "github.com/prismatic-media/prism-server/internal/api/middleware"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

func storageRouter(db *sql.DB) http.Handler {
	h := handler.NewStorageHandler(db, nil)
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.With(apimw.RequireAdmin).Get("/api/v1/storage-areas", h.ListStorage)
		r.With(apimw.RequireAdmin).Post("/api/v1/storage-areas", h.CreateStorageArea)
		r.With(apimw.RequireAdmin).Put("/api/v1/storage-areas/{id}", h.UpdateStorageArea)
		r.With(apimw.RequireAdmin).Delete("/api/v1/storage-areas/{id}", h.DeleteStorageArea)
		r.With(apimw.RequireAdmin).Put("/api/v1/storage-areas/config", h.UpdateStorageConfig)
	})
	return r
}

func TestStorageAuthRequired(t *testing.T) {
	db := openTestDB(t)
	if err := sqlite.BootstrapSettings(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/storage-areas", nil)
	w := httptest.NewRecorder()
	storageRouter(db).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d want=401", w.Code)
	}
}

func TestStorageListExcludesThumbnailArea(t *testing.T) {
	db := openTestDB(t)
	if err := sqlite.BootstrapSettings(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	admin := createUser(t, db, "admin", "a@test.com", "pass", true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/storage-areas", nil)
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, admin.ID, true))
	w := httptest.NewRecorder()
	storageRouter(db).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Areas []struct {
			Kind string `json:"kind"`
		} `json:"areas"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	for _, a := range resp.Areas {
		if a.Kind == "thumbnails" {
			t.Fatal("found unexpected thumbnail area in storage listing")
		}
	}
}

func TestStorageCreateUpdateAndConfig(t *testing.T) {
	db := openTestDB(t)
	if err := sqlite.BootstrapSettings(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	admin := createUser(t, db, "admin", "admin-storage@test.com", "pass", true)
	authz := "Bearer " + bearerToken(t, admin.ID, true)

	createBody, _ := json.Marshal(map[string]any{"kind": "segments", "path": "/tmp/segments-extra", "enabled": true})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/storage-areas", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", authz)
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	storageRouter(db).ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create code=%d body=%s", createW.Code, createW.Body.String())
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createW.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	updateBody, _ := json.Marshal(map[string]any{"path": "/tmp/segments-extra-2", "enabled": false})
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/storage-areas/"+created.ID, bytes.NewReader(updateBody))
	updateReq.Header.Set("Authorization", authz)
	updateReq.Header.Set("Content-Type", "application/json")
	updateW := httptest.NewRecorder()
	storageRouter(db).ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusOK {
		t.Fatalf("update code=%d body=%s", updateW.Code, updateW.Body.String())
	}

	cfgBody, _ := json.Marshal(map[string]string{"storage_min_free_bytes": "12345"})
	cfgReq := httptest.NewRequest(http.MethodPut, "/api/v1/storage-areas/config", bytes.NewReader(cfgBody))
	cfgReq.Header.Set("Authorization", authz)
	cfgReq.Header.Set("Content-Type", "application/json")
	cfgW := httptest.NewRecorder()
	storageRouter(db).ServeHTTP(cfgW, cfgReq)
	if cfgW.Code != http.StatusOK {
		t.Fatalf("config code=%d body=%s", cfgW.Code, cfgW.Body.String())
	}

	got, err := sqlite.GetSetting(context.Background(), db, "storage_min_free_bytes")
	if err != nil {
		t.Fatal(err)
	}
	if got != "12345" {
		t.Fatalf("storage_min_free_bytes=%q want=12345", got)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/storage-areas/"+created.ID, nil)
	deleteReq.Header.Set("Authorization", authz)
	deleteW := httptest.NewRecorder()
	storageRouter(db).ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusNoContent {
		t.Fatalf("delete code=%d want=204", deleteW.Code)
	}

	// Verify it's actually gone
	_, err = sqlite.GetStorageAreaByID(context.Background(), db, uuidParamParse(created.ID))
	if err == nil {
		t.Fatal("expected storage area to be deleted, but it still exists")
	}
}

func uuidParamParse(s string) [16]byte {
	id, _ := uuid.Parse(s)
	return id
}
