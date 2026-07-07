package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// VerificationTokenTTLHours is how long an email-verification token stays
// valid, per docs/auth_flow_spec.md ("เช่น 24 ชม.").
const VerificationTokenTTLHours = 24

// NewVerificationToken generates a random 32-byte token, hex-encoded for
// use in the verification link. Only its hash is ever stored in the DB.
func NewVerificationToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashVerificationToken returns the SHA-256 hex digest stored in
// email_verifications.token_hash. Never store the raw token.
func HashVerificationToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}
