package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	apimw "github.com/ringmaster217/galactic-media-server/internal/api/middleware"
	"github.com/ringmaster217/galactic-media-server/internal/auth"
)

const testSecret = "test-secret-key-32-bytes-long!!!"

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func bearerReq(t *testing.T, token string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

func TestAuthenticate_ValidToken(t *testing.T) {
	userID := uuid.New()
	token, err := auth.IssueAccessToken(testSecret, userID, false)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	mw := apimw.Authenticate(testSecret)(http.HandlerFunc(okHandler))
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, bearerReq(t, token))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAuthenticate_MissingHeader(t *testing.T) {
	mw := apimw.Authenticate(testSecret)(http.HandlerFunc(okHandler))
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, bearerReq(t, ""))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuthenticate_InvalidToken(t *testing.T) {
	mw := apimw.Authenticate(testSecret)(http.HandlerFunc(okHandler))
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, bearerReq(t, "not.a.valid.jwt"))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuthenticate_WrongSecret(t *testing.T) {
	userID := uuid.New()
	token, _ := auth.IssueAccessToken("other-secret", userID, false)

	mw := apimw.Authenticate(testSecret)(http.HandlerFunc(okHandler))
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, bearerReq(t, token))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuthenticate_StoresClaimsInContext(t *testing.T) {
	userID := uuid.New()
	token, _ := auth.IssueAccessToken(testSecret, userID, true)

	var gotClaims *auth.Claims
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = apimw.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	mw := apimw.Authenticate(testSecret)(handler)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, bearerReq(t, token))

	if gotClaims == nil {
		t.Fatal("expected claims in context, got nil")
	}
	if gotClaims.UserID != userID.String() {
		t.Errorf("UserID = %q, want %q", gotClaims.UserID, userID.String())
	}
	if !gotClaims.IsAdmin {
		t.Error("expected IsAdmin = true")
	}
}

func TestRequireAdmin_AdminAllowed(t *testing.T) {
	userID := uuid.New()
	token, _ := auth.IssueAccessToken(testSecret, userID, true)

	chain := apimw.Authenticate(testSecret)(apimw.RequireAdmin(http.HandlerFunc(okHandler)))
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, bearerReq(t, token))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestRequireAdmin_NonAdminForbidden(t *testing.T) {
	userID := uuid.New()
	token, _ := auth.IssueAccessToken(testSecret, userID, false)

	chain := apimw.Authenticate(testSecret)(apimw.RequireAdmin(http.HandlerFunc(okHandler)))
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, bearerReq(t, token))

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestRequireAdmin_NoClaims(t *testing.T) {
	// Call RequireAdmin directly without Authenticate in the chain.
	handler := apimw.RequireAdmin(http.HandlerFunc(okHandler))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestClaimsFromContext_NilWhenAbsent(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if claims := apimw.ClaimsFromContext(r.Context()); claims != nil {
		t.Errorf("expected nil claims, got %+v", claims)
	}
}
