package handler

import (
	"errors"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/atiroop/pdlife/internal/auth"
	"github.com/atiroop/pdlife/internal/config"
	"github.com/atiroop/pdlife/internal/mailer"
	"github.com/atiroop/pdlife/internal/models"
)

type AuthHandler struct {
	DB     *gorm.DB
	Cfg    *config.Config
	Mailer *mailer.Mailer
}

func NewAuthHandler(db *gorm.DB, cfg *config.Config, m *mailer.Mailer) *AuthHandler {
	return &AuthHandler{DB: db, Cfg: cfg, Mailer: m}
}

// ---- GET /register ----

func (h *AuthHandler) RegisterForm(c echo.Context) error {
	return c.Render(http.StatusOK, "register.html", map[string]interface{}{
		"Error": "",
	})
}

// ---- POST /register ----

func (h *AuthHandler) Register(c echo.Context) error {
	// Honeypot: a real user never fills this hidden field. Reject bots
	// quietly with the same success response, no DB writes.
	if c.FormValue("website") != "" {
		return c.Render(http.StatusOK, "register_check_email.html", nil)
	}

	email := strings.ToLower(strings.TrimSpace(c.FormValue("email")))
	password := c.FormValue("password")
	nickname := strings.TrimSpace(c.FormValue("nickname"))

	if _, err := mail.ParseAddress(email); err != nil || !strings.Contains(email, "@") {
		return c.Render(http.StatusBadRequest, "register.html", map[string]interface{}{
			"Error":    "รูปแบบอีเมลไม่ถูกต้อง",
			"Email":    email,
			"Nickname": nickname,
		})
	}
	if nickname == "" {
		return c.Render(http.StatusBadRequest, "register.html", map[string]interface{}{
			"Error":    "กรุณากรอกชื่อเล่น",
			"Email":    email,
			"Nickname": nickname,
		})
	}
	if err := auth.ValidatePasswordStrength(password); err != nil {
		return c.Render(http.StatusBadRequest, "register.html", map[string]interface{}{
			"Error":    err.Error(),
			"Email":    email,
			"Nickname": nickname,
		})
	}

	var existing models.User
	err := h.DB.Where("email = ?", email).First(&existing).Error
	if err == nil {
		return c.Render(http.StatusBadRequest, "register.html", map[string]interface{}{
			"Error":    "อีเมลนี้ถูกใช้สมัครไว้แล้ว",
			"Email":    email,
			"Nickname": nickname,
		})
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("register: lookup user failed: %v", err)
		return c.Render(http.StatusInternalServerError, "register.html", map[string]interface{}{
			"Error": "เกิดข้อผิดพลาด กรุณาลองใหม่อีกครั้ง",
		})
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		log.Printf("register: hash password failed: %v", err)
		return c.Render(http.StatusInternalServerError, "register.html", map[string]interface{}{
			"Error": "เกิดข้อผิดพลาด กรุณาลองใหม่อีกครั้ง",
		})
	}

	user := models.User{
		Email:        email,
		PasswordHash: passwordHash,
		Nickname:     nickname,
		Role:         models.RoleUnverified,
		IsActive:     true,
	}
	if err := h.DB.Create(&user).Error; err != nil {
		log.Printf("register: create user failed: %v", err)
		return c.Render(http.StatusInternalServerError, "register.html", map[string]interface{}{
			"Error": "เกิดข้อผิดพลาด กรุณาลองใหม่อีกครั้ง",
		})
	}

	if err := h.issueAndSendVerification(user); err != nil {
		log.Printf("register: send verification email failed: %v", err)
		return c.Render(http.StatusOK, "register_check_email.html", map[string]interface{}{
			"EmailWarning": true,
		})
	}

	return c.Render(http.StatusOK, "register_check_email.html", nil)
}

func (h *AuthHandler) issueAndSendVerification(user models.User) error {
	rawToken, err := auth.NewVerificationToken()
	if err != nil {
		return err
	}
	verification := models.EmailVerification{
		UserID:    user.ID,
		TokenHash: auth.HashVerificationToken(rawToken),
		ExpiresAt: time.Now().Add(auth.VerificationTokenTTLHours * time.Hour),
	}
	if err := h.DB.Create(&verification).Error; err != nil {
		return err
	}
	verifyURL := h.Cfg.AppBaseURL + "/verify-email?token=" + rawToken
	return h.Mailer.SendVerificationEmail(user.Email, mailer.VerifyEmailData{
		Nickname:  user.Nickname,
		VerifyURL: verifyURL,
	})
}

// ---- GET /verify-email ----

func (h *AuthHandler) VerifyEmail(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		return c.Render(http.StatusBadRequest, "verify_error.html", nil)
	}
	tokenHash := auth.HashVerificationToken(token)

	var verification models.EmailVerification
	err := h.DB.Where("token_hash = ? AND used_at IS NULL", tokenHash).First(&verification).Error
	if err != nil {
		return c.Render(http.StatusBadRequest, "verify_error.html", nil)
	}
	if time.Now().After(verification.ExpiresAt) {
		return c.Render(http.StatusBadRequest, "verify_error.html", nil)
	}

	var user models.User
	if err := h.DB.First(&user, verification.UserID).Error; err != nil {
		return c.Render(http.StatusInternalServerError, "verify_error.html", nil)
	}

	now := time.Now()
	txErr := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&verification).Update("used_at", now).Error; err != nil {
			return err
		}
		return tx.Model(&user).Updates(map[string]interface{}{
			"role":              models.RoleMember,
			"email_verified_at": now,
		}).Error
	})
	if txErr != nil {
		log.Printf("verify-email: update failed: %v", txErr)
		return c.Render(http.StatusInternalServerError, "verify_error.html", nil)
	}

	sessionToken, err := auth.IssueSessionToken(h.Cfg.JWTSecret, user.ID, string(models.RoleMember))
	if err != nil {
		log.Printf("verify-email: issue session failed: %v", err)
		return c.Render(http.StatusInternalServerError, "verify_error.html", nil)
	}
	c.SetCookie(&http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   strings.HasPrefix(h.Cfg.AppBaseURL, "https://"),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(auth.SessionTTL),
	})

	var profile models.PatientProfile
	if err := h.DB.Where("user_id = ?", user.ID).First(&profile).Error; err == nil && profile.ProfileCompletedAt != nil {
		return c.Redirect(http.StatusFound, "/")
	}
	return c.Redirect(http.StatusFound, "/onboarding")
}

// ---- GET /resend-verification ----

func (h *AuthHandler) ResendVerificationForm(c echo.Context) error {
	return c.Render(http.StatusOK, "resend_verification.html", map[string]interface{}{"Sent": false})
}

// ---- POST /resend-verification ----

func (h *AuthHandler) ResendVerification(c echo.Context) error {
	email := strings.ToLower(strings.TrimSpace(c.FormValue("email")))

	// Always render the same response regardless of whether the email
	// exists or is already verified, to avoid leaking account existence.
	respond := func() error {
		return c.Render(http.StatusOK, "resend_verification.html", map[string]interface{}{"Sent": true})
	}

	if _, err := mail.ParseAddress(email); err != nil {
		return respond()
	}

	var user models.User
	if err := h.DB.Where("email = ?", email).First(&user).Error; err != nil {
		return respond()
	}
	if user.Role != models.RoleUnverified {
		return respond()
	}

	// Invalidate any outstanding tokens for this user before issuing a new one.
	if err := h.DB.Model(&models.EmailVerification{}).
		Where("user_id = ? AND used_at IS NULL", user.ID).
		Update("used_at", time.Now()).Error; err != nil {
		log.Printf("resend-verification: invalidate old tokens failed: %v", err)
		return respond()
	}

	if err := h.issueAndSendVerification(user); err != nil {
		log.Printf("resend-verification: send email failed: %v", err)
	}

	return respond()
}
