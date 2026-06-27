package auth

import (
	"testing"

	"github.com/google/uuid"

	"github.com/fleet-terminal/backend/internal/config"
)

func testService() *Service {
	cfg := &config.Config{}
	cfg.JWTSecret = []byte("unit-test-secret-at-least-32-bytes-long")
	return NewService(nil, cfg, nil)
}

func TestMFASecretEncryptionRoundTrip(t *testing.T) {
	s := testService()
	plain := "JBSWY3DPEHPK3PXP"
	enc, err := s.EncryptSecret(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if string(enc) == plain {
		t.Fatal("ciphertext must not equal plaintext")
	}
	got, err := s.DecryptSecret(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("roundtrip mismatch: got %q want %q", got, plain)
	}
}

func TestMFASecretEncryptionUniqueNonce(t *testing.T) {
	s := testService()
	a, _ := s.EncryptSecret("same-secret")
	b, _ := s.EncryptSecret("same-secret")
	if string(a) == string(b) {
		t.Fatal("encryptions of the same secret must differ (random nonce)")
	}
}

func TestMFAChallengeRoundTrip(t *testing.T) {
	s := testService()
	uid := uuid.New()
	tok, err := s.IssueMFAChallenge(uid)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, err := s.ParseMFAChallenge(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != uid {
		t.Fatalf("uid mismatch: %v != %v", got, uid)
	}
}

func TestMFAChallengeRejectsAccessToken(t *testing.T) {
	s := testService()
	// An access token must not be accepted as an MFA challenge (purpose differs).
	access, _ := IssueAccessToken(s.cfg.JWTSecret, uuid.New(), uuid.New(), "u", 60_000_000_000)
	if _, err := s.ParseMFAChallenge(access); err == nil {
		t.Fatal("access token must be rejected as an mfa challenge")
	}
}

func TestValidateTOTPRejectsGarbage(t *testing.T) {
	if ValidateTOTP("JBSWY3DPEHPK3PXP", "000000") && ValidateTOTP("JBSWY3DPEHPK3PXP", "abcdef") {
		t.Fatal("static wrong codes should not both validate")
	}
}
