package auth

import (
	"strings"
	"sync"
	"time"
)

const maxFailedLoginAttempts = 5
const loginLockDuration = 15 * time.Minute

type loginAttempt struct {
	count       int
	lockedUntil time.Time
}

// LoginLimiter tracks failed login attempts per email to lock out brute
// force guessing, on top of the per-IP rate limit on the route itself.
// In-memory only (resets on restart) - acceptable for this scale.
type LoginLimiterStore struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
}

var LoginLimiter = &LoginLimiterStore{attempts: map[string]*loginAttempt{}}

func normalizeLoginKey(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (l *LoginLimiterStore) IsLocked(email string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.attempts[normalizeLoginKey(email)]
	if !ok {
		return false
	}
	return !a.lockedUntil.IsZero() && time.Now().Before(a.lockedUntil)
}

func (l *LoginLimiterStore) RecordFailure(email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := normalizeLoginKey(email)
	a, ok := l.attempts[key]
	if !ok {
		a = &loginAttempt{}
		l.attempts[key] = a
	}
	if !a.lockedUntil.IsZero() && time.Now().After(a.lockedUntil) {
		a.count = 0
		a.lockedUntil = time.Time{}
	}
	a.count++
	if a.count >= maxFailedLoginAttempts {
		a.lockedUntil = time.Now().Add(loginLockDuration)
	}
}

func (l *LoginLimiterStore) Reset(email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, normalizeLoginKey(email))
}
