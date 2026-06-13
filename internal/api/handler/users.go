package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	apimw "github.com/prismatic-media/prism-server/internal/api/middleware"
	"github.com/prismatic-media/prism-server/internal/auth"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

// UsersHandler handles user creation and profile endpoints.
type UsersHandler struct {
	db        *sql.DB
	jwtSecret string
}

func NewUsersHandler(db *sql.DB, jwtSecret string) *UsersHandler {
	return &UsersHandler{db: db, jwtSecret: jwtSecret}
}

type createUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"is_admin"`
}

// CreateUser creates a new user account.
//
// First-run rule: if no users exist the endpoint is open and the created
// account is forced to admin. Once at least one user exists, a valid admin
// JWT is required in the Authorization header.
func (h *UsersHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	count, err := sqlite.CountUsers(r.Context(), h.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not check users", err)
		return
	}

	isFirstUser := count == 0

	if !isFirstUser {
		// Require an admin JWT for subsequent user creation.
		claims := apimw.ClaimsFromContext(r.Context())
		if claims == nil || !claims.IsAdmin {
			respondError(w, http.StatusForbidden, "admin access required")
			return
		}
	}

	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Email == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "username, email, and password are required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not hash password", err)
		return
	}

	u := &models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hash,
		IsAdmin:      isFirstUser || req.IsAdmin,
	}

	if err := sqlite.CreateUser(r.Context(), h.db, u); err != nil {
		respondError(w, http.StatusConflict, "username or email already in use")
		return
	}

	u.PasswordHash = ""
	respondJSON(w, http.StatusCreated, u)
}

type updateMeRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"` // optional; omit to keep current
}

// GetMe returns the profile of the authenticated user.
func (h *UsersHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	claims := apimw.ClaimsFromContext(r.Context())
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	userID, err := uuidFromClaims(claims)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	user, err := sqlite.GetUserByID(r.Context(), h.db, userID)
	if err != nil {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}

	user.PasswordHash = ""
	respondJSON(w, http.StatusOK, user)
}

// UpdateMe updates the authenticated user's profile fields.
func (h *UsersHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	claims := apimw.ClaimsFromContext(r.Context())
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	userID, err := uuidFromClaims(claims)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	user, err := sqlite.GetUserByID(r.Context(), h.db, userID)
	if err != nil {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}

	var req updateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Password != "" {
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not hash password", err)
			return
		}
		user.PasswordHash = hash
	}

	if err := sqlite.UpdateUser(r.Context(), h.db, user); err != nil {
		respondError(w, http.StatusConflict, "username or email already in use")
		return
	}

	user.PasswordHash = ""
	respondJSON(w, http.StatusOK, user)
}

// uuidFromClaims parses the UserID string stored in JWT claims.
func uuidFromClaims(claims *auth.Claims) (uuid.UUID, error) {
	return uuid.Parse(claims.UserID)
}
