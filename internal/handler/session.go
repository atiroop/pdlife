package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/auth"
	"github.com/atiroop/pdlife/internal/models"
)

// refreshRotationGrace is how long a refresh token keeps working after it
// has been *rotated* away (never after it has been revoked for any other
// reason — see currentSession). It only needs to cover requests that were
// already in flight when the rotation happened, which is milliseconds, so
// it is short enough that a stolen token is useless long before its 30-day
// lifetime is up.
const refreshRotationGrace = 30 * time.Second

func (h *AuthHandler) cookieSecure() bool {
	return strings.HasPrefix(h.Cfg.AppBaseURL, "https://")
}

// issueSessionCookies sets the short-lived access-token cookie and a new
// long-lived refresh-token cookie (recording the refresh token, hashed,
// in refresh_tokens so it can be revoked later). Used by both
// /verify-email and /login - the one place session issuance happens.
func (h *AuthHandler) issueSessionCookies(c echo.Context, user models.User) error {
	if user.SecurityStamp == "" {
		stamp, err := auth.NewRandomToken()
		if err != nil {
			return err
		}
		if err := h.DB.Model(&user).Update("security_stamp", stamp).Error; err != nil {
			return err
		}
		user.SecurityStamp = stamp
	}

	accessToken, err := auth.IssueSessionToken(h.Cfg.JWTSecret, user.ID, string(user.Role), user.SecurityStamp)
	if err != nil {
		return err
	}
	c.SetCookie(&http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure(),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(auth.SessionTTL),
	})

	rawRefresh, err := auth.NewRandomToken()
	if err != nil {
		return err
	}
	refresh := models.RefreshToken{
		UserID:    user.ID,
		TokenHash: auth.HashToken(rawRefresh),
		ExpiresAt: time.Now().Add(auth.RefreshTokenTTL),
	}
	if err := h.DB.Create(&refresh).Error; err != nil {
		return err
	}
	c.SetCookie(&http.Cookie{
		Name:     auth.RefreshCookieName,
		Value:    rawRefresh,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure(),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(auth.RefreshTokenTTL),
	})
	return nil
}

func (h *AuthHandler) clearSessionCookies(c echo.Context) {
	expired := time.Now().Add(-time.Hour)
	c.SetCookie(&http.Cookie{
		Name: auth.SessionCookieName, Value: "", Path: "/",
		HttpOnly: true, Secure: h.cookieSecure(), SameSite: http.SameSiteLaxMode, Expires: expired,
	})
	c.SetCookie(&http.Cookie{
		Name: auth.RefreshCookieName, Value: "", Path: "/",
		HttpOnly: true, Secure: h.cookieSecure(), SameSite: http.SameSiteLaxMode, Expires: expired,
	})
}

// Keys under which currentSession memoises its result for the request.
const (
	sessionUserKey   = "pdlife_session_user"
	sessionFailedKey = "pdlife_session_failed"
)

// currentSession identifies the logged-in user for a request, resolving it
// at most once per request and caching the answer in the echo context.
//
// The caching is not just an optimisation. Resolving a session can rotate
// the refresh token, and a request that resolved twice would rotate twice
// — the second call reading the request's original (now superseded) cookie
// and only surviving on the rotation grace. With RequireSession registered
// on a route group, that is exactly what would happen on every request:
// once in the middleware, once in the handler's own guard.
func (h *AuthHandler) currentSession(c echo.Context) (*models.User, error) {
	if user, ok := c.Get(sessionUserKey).(*models.User); ok {
		return user, nil
	}
	if failed, ok := c.Get(sessionFailedKey).(bool); ok && failed {
		return nil, errNoSession
	}

	user, err := h.resolveSession(c)
	if err != nil {
		// Only cache the "no session" verdict, not transient DB errors:
		// caching those would turn a blip into a logout for the rest of the
		// request.
		if err == errNoSession {
			c.Set(sessionFailedKey, true)
		}
		return nil, err
	}
	c.Set(sessionUserKey, user)
	return user, nil
}

// resolveSession does the actual work behind currentSession. It first
// tries the short-lived access-token cookie (validating its
// security_stamp against the DB, so a password reset invalidates it
// immediately). If that's missing or invalid, it falls back to the
// refresh-token cookie: if a matching, unexpired row exists in
// refresh_tokens, it rotates the refresh token and issues a fresh access
// token transparently. This is what lets a user who closed their browser
// mid-onboarding come back later without hitting a dead end - there's no
// explicit /refresh endpoint, this happens on any authenticated request.
func (h *AuthHandler) resolveSession(c echo.Context) (*models.User, error) {
	if cookie, err := c.Cookie(auth.SessionCookieName); err == nil {
		if claims, err := auth.ParseSessionToken(h.Cfg.JWTSecret, cookie.Value); err == nil {
			var user models.User
			if err := h.DB.First(&user, claims.UserID).Error; err == nil {
				if user.SecurityStamp != "" && user.SecurityStamp == claims.Stamp {
					return &user, nil
				}
			}
		}
	}

	refreshCookie, err := c.Cookie(auth.RefreshCookieName)
	if err != nil {
		return nil, errNoSession
	}
	tokenHash := auth.HashToken(refreshCookie.Value)

	// Deliberately not filtered on revoked_at IS NULL: a revoked row still
	// has to be told apart from a missing one — see the RevokedAt check
	// below.
	var refresh models.RefreshToken
	if err := h.DB.Where("token_hash = ?", tokenHash).First(&refresh).Error; err != nil {
		return nil, errNoSession
	}
	if time.Now().After(refresh.ExpiresAt) {
		return nil, errNoSession
	}

	var user models.User
	if err := h.DB.First(&user, refresh.UserID).Error; err != nil {
		return nil, errNoSession
	}

	// Rotation isn't atomic from the browser's side: by the time one
	// request rotates this token, sibling requests carrying the same cookie
	// are already in flight, and the new cookie has not reached the browser
	// yet. Treating those as invalid logged the patient out at random —
	// three simultaneous requests with an expired access token reliably
	// killed one or two of them.
	//
	// A revoked token is therefore not automatically a rejection — but only
	// a *rotated* one gets that benefit. Logout, password change/reset,
	// account deletion and admin suspension also set revoked_at, and those
	// have to bite immediately, so they are told apart by rotated_at rather
	// than by age alone.
	if refresh.RevokedAt != nil {
		if refresh.RotatedAt == nil {
			// Deliberately ended, not superseded.
			return nil, errNoSession
		}
		if time.Since(*refresh.RotatedAt) > refreshRotationGrace {
			return nil, errNoSession
		}
		// The sibling that won the rotation is already sending the fresh
		// pair, so serve this request and leave the cookies alone.
		return &user, nil
	}

	// Rotate. The condition is what picks a single winner: two requests can
	// both read the row as unrevoked above, and only one of them may
	// revoke it. rotated_at is set in the same statement so no reader can
	// observe a revoked-but-not-yet-marked-rotated row and reject a
	// perfectly good session.
	now := time.Now()
	res := h.DB.Model(&models.RefreshToken{}).
		Where("id = ? AND revoked_at IS NULL", refresh.ID).
		Updates(map[string]interface{}{"revoked_at": now, "rotated_at": now})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		// A sibling revoked it between the read and this update. That could
		// be a rotation or a logout racing us; re-read rather than guess.
		var current models.RefreshToken
		if err := h.DB.First(&current, refresh.ID).Error; err != nil {
			return nil, errNoSession
		}
		if current.RotatedAt == nil {
			return nil, errNoSession
		}
		return &user, nil
	}

	if err := h.issueSessionCookies(c, user); err != nil {
		return nil, err
	}
	return &user, nil
}

var errNoSession = &sessionError{"no valid session"}

type sessionError struct{ msg string }

func (e *sessionError) Error() string { return e.msg }

func (h *AuthHandler) hasCompletedOnboarding(userID uint64) bool {
	var profile models.PatientProfile
	err := h.DB.Where("user_id = ?", userID).First(&profile).Error
	return err == nil && profile.ProfileCompletedAt != nil
}

// postLoginPath decides where a just-authenticated user lands: the
// onboarding wizard if their profile isn't complete yet, the health-data
// consent page if onboarding is done but consent hasn't been given (or
// was withdrawn) yet, otherwise the dashboard (which itself shows the
// right treatment-type card — see dashboard.go). Only ever called on a
// session that already passed the role==Unverified check in the caller
// (login.go's Login, or right after VerifyEmail/OnboardingSubmit mark the
// user verified/complete), so it never needs a "not verified yet" branch
// of its own.
func (h *AuthHandler) postLoginPath(userID uint64) string {
	var profile models.PatientProfile
	err := h.DB.Where("user_id = ?", userID).First(&profile).Error
	if err != nil || profile.ProfileCompletedAt == nil {
		return "/onboarding"
	}
	if profile.HealthDataConsentAt == nil {
		return "/consent"
	}
	return "/dashboard"
}
