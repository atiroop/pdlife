package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/auth"
	"github.com/atiroop/pdlife/internal/models"
)

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

// currentSession identifies the logged-in user for a request. It first
// tries the short-lived access-token cookie (validating its
// security_stamp against the DB, so a password reset invalidates it
// immediately). If that's missing or invalid, it falls back to the
// refresh-token cookie: if a matching, unrevoked, unexpired row exists in
// refresh_tokens, it rotates the refresh token and issues a fresh access
// token transparently. This is what lets a user who closed their browser
// mid-onboarding come back later without hitting a dead end - there's no
// explicit /refresh endpoint, this happens on any authenticated request.
func (h *AuthHandler) currentSession(c echo.Context) (*models.User, error) {
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

	var refresh models.RefreshToken
	if err := h.DB.Where("token_hash = ? AND revoked_at IS NULL", tokenHash).First(&refresh).Error; err != nil {
		return nil, errNoSession
	}
	if time.Now().After(refresh.ExpiresAt) {
		return nil, errNoSession
	}

	var user models.User
	if err := h.DB.First(&user, refresh.UserID).Error; err != nil {
		return nil, errNoSession
	}

	// Rotate: revoke the used refresh token and issue a fresh pair.
	if err := h.DB.Model(&refresh).Update("revoked_at", time.Now()).Error; err != nil {
		return nil, err
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

// postLoginPath decides where a just-authenticated user lands: the log
// book for their treatment type once onboarding is complete, otherwise
// the onboarding wizard. Treatment types whose log book isn't built yet
// (CAPD/HD) fall back to the landing page.
func (h *AuthHandler) postLoginPath(userID uint64) string {
	var profile models.PatientProfile
	err := h.DB.Where("user_id = ?", userID).First(&profile).Error
	if err != nil || profile.ProfileCompletedAt == nil {
		return "/onboarding"
	}
	if profile.TreatmentType != nil && *profile.TreatmentType == models.TreatmentAPD {
		return "/apd"
	}
	return "/"
}
