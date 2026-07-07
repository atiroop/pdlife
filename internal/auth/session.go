// Package auth provides password hashing, verification tokens, and a
// minimal JWT session (access token) + refresh token cookie pair.
//
// The access token is short-lived (SessionTTL) and stateless. The
// refresh token is long-lived and stored server-side (hashed) in
// refresh_tokens so it can be revoked. Session.Stamp is checked against
// users.security_stamp on every request so changing the stamp (e.g. on
// password reset) invalidates every previously issued access token
// immediately, without needing a server-side access-token blocklist.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const SessionCookieName = "pdlife_session"
const RefreshCookieName = "pdlife_refresh"
const SessionTTL = time.Hour

type SessionClaims struct {
	UserID uint64 `json:"uid"`
	Role   string `json:"role"`
	Stamp  string `json:"stamp"`
	jwt.RegisteredClaims
}

func IssueSessionToken(secret string, userID uint64, role, stamp string) (string, error) {
	claims := SessionClaims{
		UserID: userID,
		Role:   role,
		Stamp:  stamp,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(SessionTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ParseSessionToken(secret, tokenString string) (*SessionClaims, error) {
	claims := &SessionClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid session token")
	}
	return claims, nil
}
