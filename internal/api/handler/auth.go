package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/prismatic-media/prism-server/internal/auth"
	"github.com/prismatic-media/prism-server/internal/models"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
)

const (
	refreshCookieName = "refresh_token"
	refreshTokenTTL   = 30 * 24 * time.Hour
)

// AuthHandler handles login, token refresh, and logout.
type AuthHandler struct {
	db        *sql.DB
	jwtSecret string
}

func NewAuthHandler(db *sql.DB, jwtSecret string) *AuthHandler {
	return &AuthHandler{db: db, jwtSecret: jwtSecret}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresIn    int         `json:"expires_in"`
	TokenType    string      `json:"token_type"`
	User         models.User `json:"user"`
}

// Login validates credentials and issues access and refresh tokens.
// @Summary User Login
// @Description Validates user credentials and returns both JWT access and refresh tokens.
// @Tags Authentication
// @Accept json
// @Produce json
// @Param body body loginRequest true "Login credentials"
// @Success 200 {object} loginResponse
// @Failure 400 {object} map[string]string "Invalid request body or missing fields"
// @Failure 401 {object} map[string]string "Invalid credentials"
// @Router /auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}
	if req.Username == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	user, err := sqlite.GetUserByUsername(r.Context(), h.db, req.Username)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid credentials", err)
		return
	}

	if err := auth.CheckPassword(user.PasswordHash, req.Password); err != nil {
		respondError(w, http.StatusUnauthorized, "invalid credentials", err)
		return
	}

	accessToken, err := auth.IssueAccessToken(h.jwtSecret, user.ID, user.IsAdmin)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not issue token", err)
		return
	}

	rawToken, tokenHash, err := generateRefreshToken()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not generate refresh token", err)
		return
	}

	if _, err := sqlite.CreateRefreshToken(r.Context(), h.db, user.ID, tokenHash); err != nil {
		respondError(w, http.StatusInternalServerError, "could not store refresh token", err)
		return
	}

	user.PasswordHash = "" // never send the hash to clients
	respondJSON(w, http.StatusOK, loginResponse{
		AccessToken:  accessToken,
		RefreshToken: rawToken,
		ExpiresIn:    int(auth.AccessTokenTTL.Seconds()),
		TokenType:    "Bearer",
		User:         *user,
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// Refresh issues a new access token and a new refresh token using token rotation.
// @Summary Refresh Access Token
// @Description Issues a new JWT access token and rotated refresh token using the provided refresh token.
// @Tags Authentication
// @Accept json
// @Produce json
// @Param body body refreshRequest true "Refresh token payload"
// @Success 200 {object} refreshResponse
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 401 {object} map[string]string "Missing or invalid refresh token"
// @Router /auth/refresh [post]
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}
	if req.RefreshToken == "" {
		respondError(w, http.StatusUnauthorized, "missing refresh token")
		return
	}

	stored, err := sqlite.GetRefreshTokenByHash(r.Context(), h.db, hashToken(req.RefreshToken))
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid refresh token", err)
		return
	}

	if stored.Revoked || time.Now().After(stored.ExpiresAt) {
		respondError(w, http.StatusUnauthorized, "refresh token expired or revoked")
		return
	}

	user, err := sqlite.GetUserByID(r.Context(), h.db, stored.UserID)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "user not found", err)
		return
	}

	accessToken, err := auth.IssueAccessToken(h.jwtSecret, user.ID, user.IsAdmin)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not issue token", err)
		return
	}

	newRawToken, newTokenHash, err := generateRefreshToken()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not generate refresh token", err)
		return
	}

	if _, err := sqlite.CreateRefreshToken(r.Context(), h.db, user.ID, newTokenHash); err != nil {
		respondError(w, http.StatusInternalServerError, "could not store refresh token", err)
		return
	}

	_ = sqlite.RevokeRefreshToken(r.Context(), h.db, stored.ID)

	respondJSON(w, http.StatusOK, refreshResponse{
		AccessToken:  accessToken,
		RefreshToken: newRawToken,
		ExpiresIn:    int(auth.AccessTokenTTL.Seconds()),
		TokenType:    "Bearer",
	})
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Logout revokes the provided refresh token.
// @Summary Logout User
// @Description Revokes the provided active refresh token.
// @Tags Authentication
// @Accept json
// @Success 204 "Successfully logged out"
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if req.RefreshToken != "" {
		stored, err := sqlite.GetRefreshTokenByHash(r.Context(), h.db, hashToken(req.RefreshToken))
		if err == nil {
			_ = sqlite.RevokeRefreshToken(r.Context(), h.db, stored.ID)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

// generateRefreshToken returns a cryptographically random 32-byte token as a
// hex string, along with its SHA-256 hash (which is what gets stored in the DB).
func generateRefreshToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	raw = hex.EncodeToString(b)
	hash = hashToken(raw)
	return
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
