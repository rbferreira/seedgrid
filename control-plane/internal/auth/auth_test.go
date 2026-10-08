package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("s3cr3t")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" || hash == "s3cr3t" {
		t.Fatalf("invalid hash: %q", hash)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("s3cr3t")); err != nil {
		t.Errorf("correct password rejected: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("wrong")); err == nil {
		t.Error("wrong password was accepted")
	}
}

func TestGenerateToken(t *testing.T) {
	a, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	b, _ := generateToken()
	if len(a) != 64 { // 32 bytes em hex
		t.Errorf("token length = %d, want 64", len(a))
	}
	if a == b {
		t.Error("two generated tokens should not be equal")
	}
}

func TestAgentRegistrationTokenValid(t *testing.T) {
	s := NewService(nil, "secret")
	if !s.AgentRegistrationTokenValid("secret") {
		t.Error("valid token was rejected")
	}
	if s.AgentRegistrationTokenValid("other") {
		t.Error("invalid token was accepted")
	}
	if s.AgentRegistrationTokenValid("") {
		t.Error("empty token was accepted")
	}
	if NewService(nil, "").AgentRegistrationTokenValid("any") {
		t.Error("service with no token configured should reject everything")
	}
}
