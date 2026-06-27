package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// GenerateTOTP creates a new TOTP secret for an account and returns the base32
// secret plus the otpauth:// URL (rendered as a QR code by the client).
func GenerateTOTP(issuer, account string) (secret, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: issuer, AccountName: account})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// ValidateTOTP checks a 6-digit code against a base32 secret, allowing a small
// clock skew window.
func ValidateTOTP(secret, code string) bool {
	ok, err := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// --- secret encryption at rest (AES-256-GCM, key derived from the JWT secret) ---

func (s *Service) mfaKey() [32]byte { return sha256.Sum256(append([]byte("mfa:"), s.cfg.JWTSecret...)) }

// EncryptSecret encrypts a TOTP secret for storage.
func (s *Service) EncryptSecret(plain string) ([]byte, error) {
	key := s.mfaKey()
	blk, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plain), nil), nil
}

// DecryptSecret reverses EncryptSecret.
func (s *Service) DecryptSecret(enc []byte) (string, error) {
	key := s.mfaKey()
	blk, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(blk)
	if err != nil {
		return "", err
	}
	if len(enc) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := enc[:gcm.NonceSize()], enc[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// --- MFA challenge token (issued after password step, before session) ---

type mfaClaims struct {
	UserID  uuid.UUID `json:"uid"`
	Purpose string    `json:"pur"`
	jwt.RegisteredClaims
}

// IssueMFAChallenge mints a short-lived token proving the password step passed.
func (s *Service) IssueMFAChallenge(userID uuid.UUID) (string, error) {
	now := time.Now()
	claims := mfaClaims{
		UserID: userID, Purpose: "mfa",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.cfg.JWTSecret)
}

// ParseMFAChallenge validates a challenge token and returns the user id.
func (s *Service) ParseMFAChallenge(token string) (uuid.UUID, error) {
	claims := &mfaClaims{}
	t, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) {
		return s.cfg.JWTSecret, nil
	})
	if err != nil || !t.Valid || claims.Purpose != "mfa" {
		return uuid.Nil, errors.New("invalid mfa challenge")
	}
	return claims.UserID, nil
}

// VerifyUserTOTP checks a code against all of the user's confirmed TOTP secrets.
func (s *Service) VerifyUserTOTP(secrets [][]byte, code string) bool {
	for _, enc := range secrets {
		if sec, err := s.DecryptSecret(enc); err == nil && ValidateTOTP(sec, code) {
			return true
		}
	}
	return false
}
