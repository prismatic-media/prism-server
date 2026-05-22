package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestCreateUser_FirstUserBecomesAdmin(t *testing.T) {
	db := openTestDB(t)
	router := newTestRouter(t, db)

	rec := do(t, router, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{
			"username": "first",
			"email":    "first@example.com",
			"password": "pw",
			"is_admin": false, // explicitly false, should be overridden
		}),
		nil,
	)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["is_admin"] != true {
		t.Errorf("first user should be forced to admin, got is_admin = %v", resp["is_admin"])
	}
	if resp["password_hash"] != nil {
		t.Error("response must not include password_hash")
	}
}

func TestCreateUser_SecondUserRequiresAdminToken(t *testing.T) {
	db := openTestDB(t)
	router := newTestRouter(t, db)

	// Create first (admin) user.
	do(t, router, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "admin", "email": "a@x.com", "password": "pw"}),
		nil,
	)

	// Second user creation without token → 403.
	rec := do(t, router, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "second", "email": "s@x.com", "password": "pw"}),
		nil,
	)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestCreateUser_AdminCanCreateSecondUser(t *testing.T) {
	db := openTestDB(t)
	router := newTestRouter(t, db)

	first := createUser(t, db, "admin", "admin@x.com", "pw", true)
	token := bearerToken(t, first.ID, true)

	rec := do(t, router, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "second", "email": "s@x.com", "password": "pw"}),
		map[string]string{"Authorization": "Bearer " + token},
	)
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body: %s", rec.Code, rec.Body)
	}
}

func TestCreateUser_MissingFields(t *testing.T) {
	db := openTestDB(t)
	router := newTestRouter(t, db)

	rec := do(t, router, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "noemail"}),
		nil,
	)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateUser_DuplicateConflict(t *testing.T) {
	db := openTestDB(t)
	router := newTestRouter(t, db)

	// First user (bootstraps admin).
	do(t, router, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "dup", "email": "dup@x.com", "password": "pw"}),
		nil,
	)

	// Direct DB insert to bypass first-run, then try to create same username as admin.
	admin := createUser(t, db, "adm", "adm@x.com", "pw", true)
	token := bearerToken(t, admin.ID, true)

	rec := do(t, router, http.MethodPost, "/api/v1/users",
		jsonBody(map[string]any{"username": "dup", "email": "other@x.com", "password": "pw"}),
		map[string]string{"Authorization": "Bearer " + token},
	)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestGetMe_Authenticated(t *testing.T) {
	db := openTestDB(t)
	u := createUser(t, db, "frank", "frank@x.com", "pw", false)
	router := newTestRouter(t, db)

	token := bearerToken(t, u.ID, false)
	rec := do(t, router, http.MethodGet, "/api/v1/me", nil,
		map[string]string{"Authorization": "Bearer " + token},
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["username"] != "frank" {
		t.Errorf("username = %v, want frank", resp["username"])
	}
	if resp["password_hash"] != nil {
		t.Error("response must not include password_hash")
	}
}

func TestGetMe_Unauthenticated(t *testing.T) {
	db := openTestDB(t)
	router := newTestRouter(t, db)

	rec := do(t, router, http.MethodGet, "/api/v1/me", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestUpdateMe_ChangesUsername(t *testing.T) {
	db := openTestDB(t)
	u := createUser(t, db, "grace", "grace@x.com", "pw", false)
	router := newTestRouter(t, db)

	token := bearerToken(t, u.ID, false)
	rec := do(t, router, http.MethodPut, "/api/v1/me",
		jsonBody(map[string]any{"username": "grace2"}),
		map[string]string{"Authorization": "Bearer " + token},
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["username"] != "grace2" {
		t.Errorf("username = %v, want grace2", resp["username"])
	}
}

func TestUpdateMe_ChangesPassword(t *testing.T) {
	db := openTestDB(t)
	u := createUser(t, db, "henry", "henry@x.com", "oldpw", false)
	router := newTestRouter(t, db)

	token := bearerToken(t, u.ID, false)
	rec := do(t, router, http.MethodPut, "/api/v1/me",
		jsonBody(map[string]any{"password": "newpw"}),
		map[string]string{"Authorization": "Bearer " + token},
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// Verify new password works via login.
	loginRec := do(t, router, http.MethodPost, "/api/v1/auth/login",
		jsonBody(map[string]string{"username": "henry", "password": "newpw"}),
		nil,
	)
	if loginRec.Code != http.StatusOK {
		t.Errorf("login with new password status = %d, want 200", loginRec.Code)
	}
}

func TestUpdateMe_Unauthenticated(t *testing.T) {
	db := openTestDB(t)
	router := newTestRouter(t, db)

	rec := do(t, router, http.MethodPut, "/api/v1/me",
		jsonBody(map[string]any{"username": "x"}),
		nil,
	)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
