package middleware_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	apimw "github.com/prismatic-media/prism-server/internal/api/middleware"
	"github.com/prismatic-media/prism-server/internal/auth"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("sqlite.Migrate: %v", err)
	}
	if err := sqlite.BootstrapSettings(context.Background(), db); err != nil {
		t.Fatalf("BootstrapSettings: %v", err)
	}
	return db
}

func TestSetupGuard(t *testing.T) {
	db := setupTestDB(t)

	// A simple target handler that returns 200 OK.
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	guard := apimw.SetupGuard(db)(dummyHandler)

	tests := []struct {
		name           string
		setupComplete  bool
		method         string
		path           string
		expectedStatus int
	}{
		// Under setup not complete:
		{
			name:           "Setup incomplete: normal API endpoint is blocked",
			setupComplete:  false,
			method:         http.MethodGet,
			path:           "/api/v1/users",
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:           "Setup incomplete: browser route redirects to setup",
			setupComplete:  false,
			method:         http.MethodGet,
			path:           "/movies",
			expectedStatus: http.StatusTemporaryRedirect,
		},
		{
			name:           "Setup incomplete: setup page is allowed",
			setupComplete:  false,
			method:         http.MethodGet,
			path:           "/setup",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Setup incomplete: api setup endpoint is allowed",
			setupComplete:  false,
			method:         http.MethodPost,
			path:           "/api/v1/setup",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Setup incomplete: fs browse api endpoint is allowed",
			setupComplete:  false,
			method:         http.MethodGet,
			path:           "/api/v1/fs:browse",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Setup incomplete: static assets are allowed",
			setupComplete:  false,
			method:         http.MethodGet,
			path:           "/main.js",
			expectedStatus: http.StatusOK,
		},

		// Under setup complete:
		{
			name:           "Setup complete: normal API endpoint is allowed",
			setupComplete:  true,
			method:         http.MethodGet,
			path:           "/api/v1/users",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Setup complete: browser route is allowed",
			setupComplete:  true,
			method:         http.MethodGet,
			path:           "/movies",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := "false"
			if tt.setupComplete {
				status = "true"
			}
			err := sqlite.SetSetting(context.Background(), db, "setup_complete", status)
			if err != nil {
				t.Fatalf("failed to set setup_complete: %v", err)
			}

			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			guard.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d for path %s (setup complete: %t)", tt.expectedStatus, rec.Code, tt.path, tt.setupComplete)
			}
		})
	}
}

func TestAuthenticateSetupOrAdmin(t *testing.T) {
	db := setupTestDB(t)

	// A simple target handler that returns 200 OK.
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// AuthenticateSetupOrAdmin requires db and jwtSecret
	mw := apimw.AuthenticateSetupOrAdmin(db, testSecret)(dummyHandler)

	// Case 1: Setup is not complete -> should allow unauthenticated access
	err := sqlite.SetSetting(context.Background(), db, "setup_complete", "false")
	if err != nil {
		t.Fatalf("failed to set setup_complete: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/fs:browse", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Setup incomplete: expected unauthenticated access to be allowed, got status %d", rec.Code)
	}

	// Case 2: Setup is complete -> should require admin authentication
	err = sqlite.SetSetting(context.Background(), db, "setup_complete", "true")
	if err != nil {
		t.Fatalf("failed to set setup_complete: %v", err)
	}

	// Sub-case 2a: No token -> 401
	req = httptest.NewRequest(http.MethodGet, "/api/v1/fs:browse", nil)
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Setup complete, no token: expected 401 Unauthorized, got status %d", rec.Code)
	}

	// Sub-case 2b: Non-admin token -> 403
	nonAdminTok, err := auth.IssueAccessToken(testSecret, uuid.New(), false)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/fs:browse", nil)
	req.Header.Set("Authorization", "Bearer "+nonAdminTok)
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Setup complete, non-admin token: expected 403 Forbidden, got status %d", rec.Code)
	}

	// Sub-case 2c: Admin token -> 200
	adminTok, err := auth.IssueAccessToken(testSecret, uuid.New(), true)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/fs:browse", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Setup complete, admin token: expected 200 OK, got status %d", rec.Code)
	}
}
