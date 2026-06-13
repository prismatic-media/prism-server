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
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/ringmaster217/prism/internal/api/handler"
	apimw "github.com/ringmaster217/prism/internal/api/middleware"
	"github.com/ringmaster217/prism/internal/auth"
	"github.com/ringmaster217/prism/internal/models"
	"github.com/ringmaster217/prism/internal/store/sqlite"
)

const testSecret = "test-secret-key-32-bytes-long!!!"

// newTestRouter wires auth + user handlers the same way the production router
// does, but backed by the supplied in-memory DB.
func newTestRouter(t *testing.T, db *sql.DB) http.Handler {
	t.Helper()
	authH := handler.NewAuthHandler(db, testSecret)
	userH := handler.NewUsersHandler(db, testSecret)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)

	r.Post("/api/v1/auth/login", authH.Login)
	r.Post("/api/v1/auth/refresh", authH.Refresh)
	r.Post("/api/v1/auth/logout", authH.Logout)
	r.With(apimw.OptionalAuthenticate(testSecret)).Post("/api/v1/users", userH.CreateUser)

	r.Group(func(r chi.Router) {
		r.Use(apimw.Authenticate(testSecret))
		r.Get("/api/v1/me", userH.GetMe)
		r.Put("/api/v1/me", userH.UpdateMe)
	})

	return r
}

// openTestDB opens an in-memory SQLite DB with migrations applied.
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
	return db
}

// createUser creates a user row and returns the model (with ID set).
func createUser(t *testing.T, db *sql.DB, username, email, password string, isAdmin bool) *models.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	u := &models.User{Username: username, Email: email, PasswordHash: hash, IsAdmin: isAdmin}
	if err := sqlite.CreateUser(context.Background(), db, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u
}

// bearerToken issues a signed JWT for the given user.
func bearerToken(t *testing.T, userID uuid.UUID, isAdmin bool) string {
	t.Helper()
	tok, err := auth.IssueAccessToken(testSecret, userID, isAdmin)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	return tok
}

// jsonBody encodes v as a JSON io.Reader.
func jsonBody(v any) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

// do runs a request against the provided handler and decodes the JSON response.
func do(t *testing.T, h http.Handler, method, path string, body *bytes.Buffer, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		body = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
