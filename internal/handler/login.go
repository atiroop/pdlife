package handler

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/atiroop/pdlife/internal/auth"
	"github.com/atiroop/pdlife/internal/mailer"
	"github.com/atiroop/pdlife/internal/models"
)

// ---- GET /login ----

func (h *AuthHandler) LoginForm(c echo.Context) error {
	return c.Render(http.StatusOK, "login.html", map[string]interface{}{"Error": ""})
}

// ---- POST /login ----

func (h *AuthHandler) Login(c echo.Context) error {
	email := strings.ToLower(strings.TrimSpace(c.FormValue("email")))
	password := c.FormValue("password")

	renderError := func(msg string) error {
		return c.Render(http.StatusUnauthorized, "login.html", map[string]interface{}{
			"Error": msg,
			"Email": email,
		})
	}

	if email == "" || password == "" {
		return renderError("กรุณากรอกอีเมลและรหัสผ่าน")
	}

	if auth.LoginLimiter.IsLocked(email) {
		return c.Render(http.StatusTooManyRequests, "login.html", map[string]interface{}{
			"Error": "เข้าสู่ระบบผิดหลายครั้งเกินไป กรุณาลองใหม่ภายใน 15 นาที",
			"Email": email,
		})
	}

	var user models.User
	if err := h.DB.Where("email = ?", email).First(&user).Error; err != nil {
		auth.LoginLimiter.RecordFailure(email)
		return renderError("อีเมลหรือรหัสผ่านไม่ถูกต้อง")
	}

	if user.Role == models.RoleUnverified {
		return c.Render(http.StatusUnauthorized, "login_unverified.html", map[string]interface{}{
			"Email": email,
		})
	}

	if !auth.CheckPassword(user.PasswordHash, password) {
		auth.LoginLimiter.RecordFailure(email)
		return renderError("อีเมลหรือรหัสผ่านไม่ถูกต้อง")
	}

	auth.LoginLimiter.Reset(email)

	now := time.Now()
	if err := h.DB.Model(&user).Update("last_login_at", now).Error; err != nil {
		log.Printf("login: update last_login_at failed: %v", err)
	}

	if err := h.issueSessionCookies(c, user); err != nil {
		log.Printf("login: issue session failed: %v", err)
		return renderError("เกิดข้อผิดพลาด กรุณาลองใหม่อีกครั้ง")
	}

	if h.hasCompletedOnboarding(user.ID) {
		return c.Redirect(http.StatusFound, "/")
	}
	return c.Redirect(http.StatusFound, "/onboarding")
}

// ---- POST /logout ----

func (h *AuthHandler) Logout(c echo.Context) error {
	if cookie, err := c.Cookie(auth.RefreshCookieName); err == nil {
		h.DB.Model(&models.RefreshToken{}).
			Where("token_hash = ?", auth.HashToken(cookie.Value)).
			Update("revoked_at", time.Now())
	}
	h.clearSessionCookies(c)
	return c.Redirect(http.StatusFound, "/")
}

// ---- GET /forgot-password ----

func (h *AuthHandler) ForgotPasswordForm(c echo.Context) error {
	return c.Render(http.StatusOK, "forgot_password.html", map[string]interface{}{"Sent": false})
}

// ---- POST /forgot-password ----

func (h *AuthHandler) ForgotPassword(c echo.Context) error {
	email := strings.ToLower(strings.TrimSpace(c.FormValue("email")))

	// Same response regardless of whether the account exists, to avoid
	// leaking account existence - matches /resend-verification.
	respond := func() error {
		return c.Render(http.StatusOK, "forgot_password.html", map[string]interface{}{"Sent": true})
	}

	var user models.User
	if err := h.DB.Where("email = ?", email).First(&user).Error; err != nil {
		return respond()
	}

	rawToken, err := auth.NewRandomToken()
	if err != nil {
		log.Printf("forgot-password: generate token failed: %v", err)
		return respond()
	}
	reset := models.PasswordResetToken{
		UserID:    user.ID,
		TokenHash: auth.HashToken(rawToken),
		ExpiresAt: time.Now().Add(auth.ResetTokenTTL),
	}
	if err := h.DB.Create(&reset).Error; err != nil {
		log.Printf("forgot-password: create token failed: %v", err)
		return respond()
	}

	resetURL := h.Cfg.AppBaseURL + "/reset-password?token=" + rawToken
	if err := h.Mailer.SendPasswordResetEmail(user.Email, mailer.ResetPasswordData{
		Nickname: user.Nickname,
		ResetURL: resetURL,
	}); err != nil {
		log.Printf("forgot-password: send email failed: %v", err)
	}

	return respond()
}

// ---- GET /reset-password ----

func (h *AuthHandler) ResetPasswordForm(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" || !h.resetTokenValid(token) {
		return c.Render(http.StatusBadRequest, "reset_password_error.html", nil)
	}
	return c.Render(http.StatusOK, "reset_password.html", map[string]interface{}{
		"Token": token,
		"Error": "",
	})
}

func (h *AuthHandler) resetTokenValid(token string) bool {
	var reset models.PasswordResetToken
	err := h.DB.Where("token_hash = ? AND used_at IS NULL", auth.HashToken(token)).First(&reset).Error
	return err == nil && time.Now().Before(reset.ExpiresAt)
}

// ---- POST /reset-password ----

func (h *AuthHandler) ResetPassword(c echo.Context) error {
	token := c.FormValue("token")
	newPassword := c.FormValue("password")

	var reset models.PasswordResetToken
	err := h.DB.Where("token_hash = ? AND used_at IS NULL", auth.HashToken(token)).First(&reset).Error
	if err != nil || time.Now().After(reset.ExpiresAt) {
		return c.Render(http.StatusBadRequest, "reset_password_error.html", nil)
	}

	if err := auth.ValidatePasswordStrength(newPassword); err != nil {
		return c.Render(http.StatusBadRequest, "reset_password.html", map[string]interface{}{
			"Token": token,
			"Error": err.Error(),
		})
	}

	var user models.User
	if err := h.DB.First(&user, reset.UserID).Error; err != nil {
		return c.Render(http.StatusInternalServerError, "reset_password_error.html", nil)
	}

	passwordHash, err := auth.HashPassword(newPassword)
	if err != nil {
		log.Printf("reset-password: hash password failed: %v", err)
		return c.Render(http.StatusInternalServerError, "reset_password.html", map[string]interface{}{
			"Token": token,
			"Error": "เกิดข้อผิดพลาด กรุณาลองใหม่อีกครั้ง",
		})
	}

	newStamp, err := auth.NewRandomToken()
	if err != nil {
		log.Printf("reset-password: generate stamp failed: %v", err)
		return c.Render(http.StatusInternalServerError, "reset_password.html", map[string]interface{}{
			"Token": token,
			"Error": "เกิดข้อผิดพลาด กรุณาลองใหม่อีกครั้ง",
		})
	}

	now := time.Now()
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&reset).Update("used_at", now).Error; err != nil {
			return err
		}
		if err := tx.Model(&user).Updates(map[string]interface{}{
			"password_hash":  passwordHash,
			"security_stamp": newStamp, // invalidates every previously issued JWT at once
		}).Error; err != nil {
			return err
		}
		// Force re-login everywhere: revoke all outstanding refresh tokens too.
		return tx.Model(&models.RefreshToken{}).
			Where("user_id = ? AND revoked_at IS NULL", user.ID).
			Update("revoked_at", now).Error
	}); err != nil {
		log.Printf("reset-password: update failed: %v", err)
		return c.Render(http.StatusInternalServerError, "reset_password_error.html", nil)
	}

	h.clearSessionCookies(c)

	return c.Render(http.StatusOK, "reset_password_complete.html", nil)
}
