package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAccessTokenRoundTrip(t *testing.T) {
	secret := []byte("test-secret-at-least-32-bytes-long-xx")
	uid, sid := uuid.New(), uuid.New()
	tok, err := IssueAccessToken(secret, uid, sid, "alice", time.Minute)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := ParseAccessToken(secret, tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.UserID != uid || claims.SessionID != sid || claims.Username != "alice" {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}

func TestAccessTokenWrongSecret(t *testing.T) {
	tok, _ := IssueAccessToken([]byte("secret-one-aaaaaaaaaaaaaaaaaaaaaaaa"), uuid.New(), uuid.New(), "bob", time.Minute)
	if _, err := ParseAccessToken([]byte("secret-two-bbbbbbbbbbbbbbbbbbbbbbbb"), tok); err == nil {
		t.Fatal("expected verification failure with wrong secret")
	}
}

func TestAccessTokenExpired(t *testing.T) {
	secret := []byte("test-secret-at-least-32-bytes-long-xx")
	tok, _ := IssueAccessToken(secret, uuid.New(), uuid.New(), "carol", -time.Minute)
	if _, err := ParseAccessToken(secret, tok); err == nil {
		t.Fatal("expected expired token to fail")
	}
}

func TestRefreshTokenHashing(t *testing.T) {
	tok, hash, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("new refresh: %v", err)
	}
	if tok == hash {
		t.Fatal("token and hash must differ")
	}
	if HashToken(tok) != hash {
		t.Fatal("HashToken must reproduce the stored hash")
	}
}
