package handler_test

import (
	"encoding/json"
	"net/http"
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
	if resp["refresh_token"] == "" {
		t.Error("expected non-empty refresh_token")
	}
	if resp["token_type"] != "Bearer" {
		t.Errorf("token_type = %v, want Bearer", resp["token_type"])
	}
	if _, ok := resp["expires_in"]; !ok {
		t.Error("expected expires_in to be present")
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

func TestRefresh_ValidToken(t *testing.T) {
	db := openTestDB(t)
	createUser(t, db, "dave", "dave@example.com", "pw", false)
	router := newTestRouter(t, db)

	// Login to get the token.
	loginRec := do(t, router, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "dave", "password": "pw"}),
		nil,
	)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d", loginRec.Code)
	}

	var loginResp map[string]any
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	refreshToken, ok := loginResp["refresh_token"].(string)
	if !ok || refreshToken == "" {
		t.Fatal("no refresh_token in login response")
	}

	// Use the token to refresh.
	rec := do(t, router, http.MethodPost, "/api/v1/auth/refresh",
		jsonBody(map[string]string{"refresh_token": refreshToken}),
		nil,
	)
	if rec.Code != http.StatusOK {
		t.Errorf("refresh status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["access_token"] == "" {
		t.Error("expected access_token in refresh response")
	}
	newRefreshToken := resp["refresh_token"].(string)
	if newRefreshToken == "" || newRefreshToken == refreshToken {
		t.Error("expected a newly rotated refresh_token in refresh response")
	}
}

func TestRefresh_MissingToken(t *testing.T) {
	db := openTestDB(t)
	router := newTestRouter(t, db)

	rec := do(t, router, http.MethodPost, "/api/v1/auth/refresh", jsonBody(map[string]string{}), nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestLogout_RevokesToken(t *testing.T) {
	db := openTestDB(t)
	createUser(t, db, "eve", "eve@example.com", "pw", false)
	router := newTestRouter(t, db)

	loginRec := do(t, router, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "eve", "password": "pw"}),
		nil,
	)
	var loginResp map[string]any
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	refreshToken := loginResp["refresh_token"].(string)

	// Logout.
	logoutRec := do(t, router, http.MethodPost, "/api/v1/auth/logout",
		jsonBody(map[string]string{"refresh_token": refreshToken}),
		nil,
	)
	if logoutRec.Code != http.StatusNoContent {
		t.Errorf("logout status = %d, want 204", logoutRec.Code)
	}

	// After logout, refresh should be rejected.
	refreshRec := do(t, router, http.MethodPost, "/api/v1/auth/refresh",
		jsonBody(map[string]string{"refresh_token": refreshToken}),
		nil,
	)
	if refreshRec.Code != http.StatusUnauthorized {
		t.Errorf("post-logout refresh status = %d, want 401", refreshRec.Code)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
