// Package auth provides password hashing, verification tokens, and a
// minimal JWT session cookie.
//
// The full login/refresh-token endpoints described in
// docs/auth_flow_spec.md are a separate follow-up task. This session
// cookie exists only so a user can be identified for the onboarding
// wizard immediately after verifying their email, without asking them
// to log in again.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const SessionCookieName = "pdlife_session"
const SessionTTL = time.Hour

type SessionClaims struct {
	UserID uint64 `json:"uid"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func IssueSessionToken(secret string, userID uint64, role string) (string, error) {
	claims := SessionClaims{
		UserID: userID,
		Role:   role,
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
