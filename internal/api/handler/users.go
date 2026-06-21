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
// @Summary Create User (First-run or Admin Only)
// @Description Create a new user account. If no users exist, this request is open and makes the user an Admin. Otherwise, it requires Admin JWT.
// @Tags User Profile
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body createUserRequest true "User creation payload"
// @Success 201 {object} models.User
// @Failure 400 {object} map[string]string "Invalid request body or missing fields"
// @Failure 403 {object} map[string]string "Forbidden (requires Admin status)"
// @Failure 409 {object} map[string]string "Username or email already in use"
// @Router /users [post]
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
		respondError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}
	if req.Username == "" || req.Email == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "username, email, and password are required", err)
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
		respondError(w, http.StatusConflict, "username or email already in use", err)
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
// @Summary Get Current User Profile
// @Description Returns profile details for the currently logged-in user.
// @Tags User Profile
// @Security BearerAuth
// @Produce json
// @Success 200 {object} models.User
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 404 {object} map[string]string "User not found"
// @Router /me [get]
func (h *UsersHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	claims := apimw.ClaimsFromContext(r.Context())
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	userID, err := uuidFromClaims(claims)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token", err)
		return
	}

	user, err := sqlite.GetUserByID(r.Context(), h.db, userID)
	if err != nil {
		respondError(w, http.StatusNotFound, "user not found", err)
		return
	}

	user.PasswordHash = ""
	respondJSON(w, http.StatusOK, user)
}

// UpdateMe updates the authenticated user's profile fields.
// @Summary Update Current User Profile
// @Description Allows the authenticated user to update their profile details (username, email, password).
// @Tags User Profile
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body updateMeRequest true "User update payload"
// @Success 200 {object} models.User
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 401 {object} map[string]string "Unauthenticated"
// @Failure 404 {object} map[string]string "User not found"
// @Failure 409 {object} map[string]string "Username or email already in use"
// @Router /me [put]
func (h *UsersHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	claims := apimw.ClaimsFromContext(r.Context())
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	userID, err := uuidFromClaims(claims)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token", err)
		return
	}

	user, err := sqlite.GetUserByID(r.Context(), h.db, userID)
	if err != nil {
		respondError(w, http.StatusNotFound, "user not found", err)
		return
	}

	var req updateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", err)
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
		respondError(w, http.StatusConflict, "username or email already in use", err)
		return
	}

	user.PasswordHash = ""
	respondJSON(w, http.StatusOK, user)
}

// uuidFromClaims parses the UserID string stored in JWT claims.
func uuidFromClaims(claims *auth.Claims) (uuid.UUID, error) {
	return uuid.Parse(claims.UserID)
}
