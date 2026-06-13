package auth_test

import (
	"testing"

	"github.com/prismatic-media/prism-server/internal/auth"
)

func TestHashPassword_ProducesNonEmptyHash(t *testing.T) {
	hash, err := auth.HashPassword("secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if hash == "secret" {
		t.Fatal("hash must not equal plaintext")
	}
}

func TestHashPassword_DifferentSaltsEachCall(t *testing.T) {
	h1, _ := auth.HashPassword("same")
	h2, _ := auth.HashPassword("same")
	if h1 == h2 {
		t.Error("two hashes of the same password should differ (different salts)")
	}
}

func TestCheckPassword_CorrectPassword(t *testing.T) {
	hash, err := auth.HashPassword("hunter2")
	if err != nil {
		t.Fatalf("hash error: %v", err)
	}
	if err := auth.CheckPassword(hash, "hunter2"); err != nil {
		t.Errorf("expected nil error for correct password, got: %v", err)
	}
}

func TestCheckPassword_WrongPassword(t *testing.T) {
	hash, err := auth.HashPassword("hunter2")
	if err != nil {
		t.Fatalf("hash error: %v", err)
	}
	if err := auth.CheckPassword(hash, "wrong"); err == nil {
		t.Error("expected error for wrong password, got nil")
	}
}

func TestCheckPassword_EmptyPassword(t *testing.T) {
	hash, _ := auth.HashPassword("notempty")
	if err := auth.CheckPassword(hash, ""); err == nil {
		t.Error("expected error for empty password check, got nil")
	}
}
