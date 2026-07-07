package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// VerificationTokenTTLHours is how long an email-verification token stays
// valid, per docs/auth_flow_spec.md ("เช่น 24 ชม.").
const VerificationTokenTTLHours = 24

// ResetTokenTTL is how long a password-reset token stays valid - shorter
// than email verification since a reset should happen promptly.
const ResetTokenTTL = 1 * time.Hour

// RefreshTokenTTL is how long a refresh token (long-lived, server-side
// revocable) stays valid.
const RefreshTokenTTL = 30 * 24 * time.Hour

// NewRandomToken generates a random 32-byte token, hex-encoded for use in
// links or cookies. Only its hash is ever stored in the DB.
func NewRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashToken returns the SHA-256 hex digest of a raw token, as stored in
// the various *_token_hash DB columns. Never store the raw token.
func HashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// NewVerificationToken and HashVerificationToken are kept as named
// wrappers around the generic helpers above for readability at call sites
// that deal specifically with email verification.
func NewVerificationToken() (string, error)        { return NewRandomToken() }
func HashVerificationToken(rawToken string) string { return HashToken(rawToken) }
