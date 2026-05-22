package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogin_Success(t *testing.T) {
	db := openTestDB(t)
	createUser(t, db, "alice", "alice@example.com", "pass123", false)
	router := newTestRouter(t, db)

	rec := do(t, router, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "alice", "password": "pass123"}),
		nil,
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["access_token"] == "" {
		t.Error("expected non-empty access_token")
	}
	// Refresh cookie must be set.
	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == "refresh_token" {
			found = true
			if !c.HttpOnly {
				t.Error("refresh_token cookie should be HttpOnly")
			}
		}
	}
	if !found {
		t.Error("expected refresh_token cookie to be set")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	db := openTestDB(t)
	createUser(t, db, "bob", "bob@example.com", "correct", false)
	router := newTestRouter(t, db)

	rec := do(t, router, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "bob", "password": "wrong"}),
		nil,
	)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestLogin_UnknownUser(t *testing.T) {
	db := openTestDB(t)
	router := newTestRouter(t, db)

	rec := do(t, router, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "ghost", "password": "pw"}),
		nil,
	)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestLogin_MissingFields(t *testing.T) {
	db := openTestDB(t)
	router := newTestRouter(t, db)

	rec := do(t, router, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "alice"}),
		nil,
	)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestLogin_PasswordNotInResponse(t *testing.T) {
	db := openTestDB(t)
	createUser(t, db, "carol", "carol@example.com", "pw", false)
	router := newTestRouter(t, db)

	rec := do(t, router, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "carol", "password": "pw"}),
		nil,
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	// password_hash must never appear in any response.
	if contains(body, "password_hash") || contains(body, "PasswordHash") {
		t.Error("response must not contain password hash")
	}
}

// TestRefresh_ValidCookie confirms a valid refresh cookie yields a new access token.
func TestRefresh_ValidCookie(t *testing.T) {
	db := openTestDB(t)
	createUser(t, db, "dave", "dave@example.com", "pw", false)
	router := newTestRouter(t, db)

	// Login to get the cookie.
	loginRec := do(t, router, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "dave", "password": "pw"}),
		nil,
	)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d", loginRec.Code)
	}
	var cookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == "refresh_token" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no refresh_token cookie after login")
	}

	// Use the cookie to refresh.
	rec := execWithCookie(t, router, http.MethodPost, "/api/v1/auth/refresh", cookie)
	if rec.Code != http.StatusOK {
		t.Errorf("refresh status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["access_token"] == "" {
		t.Error("expected access_token in refresh response")
	}
}

func TestRefresh_NoCookie(t *testing.T) {
	db := openTestDB(t)
	router := newTestRouter(t, db)

	rec := do(t, router, http.MethodPost, "/api/v1/auth/refresh", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestLogout_RevokesCookie(t *testing.T) {
	db := openTestDB(t)
	createUser(t, db, "eve", "eve@example.com", "pw", false)
	router := newTestRouter(t, db)

	loginRec := do(t, router, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "eve", "password": "pw"}),
		nil,
	)
	var cookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == "refresh_token" {
			cookie = c
		}
	}

	// Logout.
	logoutRec := execWithCookie(t, router, http.MethodPost, "/api/v1/auth/logout", cookie)
	if logoutRec.Code != http.StatusNoContent {
		t.Errorf("logout status = %d, want 204", logoutRec.Code)
	}

	// After logout, refresh should be rejected.
	refreshRec := execWithCookie(t, router, http.MethodPost, "/api/v1/auth/refresh", cookie)
	if refreshRec.Code != http.StatusUnauthorized {
		t.Errorf("post-logout refresh status = %d, want 401", refreshRec.Code)
	}
}

// execWithCookie sends a request with a single cookie attached.
func execWithCookie(t *testing.T, h http.Handler, method, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
