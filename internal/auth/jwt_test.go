package auth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/ringmaster217/prism/internal/auth"
)

const testSecret = "test-secret-key-32-bytes-long!!!"

func TestIssueAccessToken_ValidClaims(t *testing.T) {
	userID := uuid.New()
	token, err := auth.IssueAccessToken(testSecret, userID, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := auth.ValidateAccessToken(testSecret, token)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if claims.UserID != userID.String() {
		t.Errorf("UserID = %q, want %q", claims.UserID, userID.String())
	}
	if !claims.IsAdmin {
		t.Error("expected IsAdmin = true")
	}
}

func TestIssueAccessToken_NonAdminUser(t *testing.T) {
	userID := uuid.New()
	token, err := auth.IssueAccessToken(testSecret, userID, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claims, err := auth.ValidateAccessToken(testSecret, token)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if claims.IsAdmin {
		t.Error("expected IsAdmin = false")
	}
}

func TestValidateAccessToken_WrongSecret(t *testing.T) {
	userID := uuid.New()
	token, err := auth.IssueAccessToken(testSecret, userID, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = auth.ValidateAccessToken("wrong-secret", token)
	if err == nil {
		t.Fatal("expected error for wrong secret, got nil")
	}
}

func TestValidateAccessToken_Expired(t *testing.T) {
	userID := uuid.New()
	claims := auth.Claims{
		UserID:  userID.String(),
		IsAdmin: false,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("could not sign token: %v", err)
	}

	_, err = auth.ValidateAccessToken(testSecret, signed)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestValidateAccessToken_Tampered(t *testing.T) {
	userID := uuid.New()
	token, err := auth.IssueAccessToken(testSecret, userID, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Flip the last character of the signature.
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal("unexpected token format")
	}
	last := parts[2]
	if last[len(last)-1] == 'A' {
		parts[2] = last[:len(last)-1] + "B"
	} else {
		parts[2] = last[:len(last)-1] + "A"
	}
	tampered := strings.Join(parts, ".")

	_, err = auth.ValidateAccessToken(testSecret, tampered)
	if err == nil {
		t.Fatal("expected error for tampered token, got nil")
	}
}

func TestValidateAccessToken_WrongAlgorithm(t *testing.T) {
	userID := uuid.New()
	// Sign with RS256 (no private key, just construct malformed header).
	claims := auth.Claims{
		UserID:  userID.String(),
		IsAdmin: false,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	// Use "none" algorithm by manually building a token-shaped string.
	// We just verify the validator rejects anything not signed with HS256.
	header := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0" // {"alg":"none","typ":"JWT"}
	payloadToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := payloadToken.SignedString([]byte(testSecret))
	payloadParts := strings.Split(signed, ".")
	noneToken := header + "." + payloadParts[1] + "."

	_, err := auth.ValidateAccessToken(testSecret, noneToken)
	if err == nil {
		t.Fatal("expected error for none-algorithm token, got nil")
	}
}

func TestValidateAccessToken_EmptyString(t *testing.T) {
	_, err := auth.ValidateAccessToken(testSecret, "")
	if err == nil {
		t.Fatal("expected error for empty token string, got nil")
	}
}
