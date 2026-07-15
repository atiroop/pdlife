package handler

import (
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/models"
)

// HealthDataConsentVersion is stamped onto PatientProfile.HealthDataConsentVersion
// whenever a user gives consent, and LegalContentUpdatedDate (below) is the
// Thai-formatted display of the same date — so a future revision of the
// privacy policy just needs a new consent version, not a text diff, to
// tell which users consented under which policy revision.
const HealthDataConsentVersion = "2026-07-10"

// LegalContentUpdatedDate is the "ปรับปรุงล่าสุด" date shown on
// /terms, /privacy, /cookie-policy — keep in sync with HealthDataConsentVersion.
const LegalContentUpdatedDate = "10 กรกฎาคม 2569"

// ---- GET /consent — retroactive consent for users who completed
// onboarding before this feature existed. New users never see this page:
// OnboardingSubmit collects consent as part of the onboarding form
// itself, in the same request that sets ProfileCompletedAt. ----

func (h *AuthHandler) ConsentForm(c echo.Context) error {
	user, err := h.currentSession(c)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/login")
	}
	if user.Role == models.RoleUnverified {
		return c.Render(http.StatusForbidden, "placeholder.html", map[string]string{
			"Title":   "ยืนยันอีเมลก่อน",
			"Message": "กรุณายืนยันอีเมลก่อนใช้งาน",
		})
	}

	var profile models.PatientProfile
	if err := h.DB.Where("user_id = ?", user.ID).First(&profile).Error; err != nil || profile.ProfileCompletedAt == nil {
		return c.Redirect(http.StatusSeeOther, "/onboarding")
	}
	if profile.HealthDataConsentAt != nil {
		return c.Redirect(http.StatusSeeOther, h.postLoginPath(user.ID))
	}

	return c.Render(http.StatusOK, "consent.html", map[string]interface{}{"Error": ""})
}

func (h *AuthHandler) ConsentSubmit(c echo.Context) error {
	user, err := h.currentSession(c)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/login")
	}

	var profile models.PatientProfile
	if err := h.DB.Where("user_id = ?", user.ID).First(&profile).Error; err != nil || profile.ProfileCompletedAt == nil {
		return c.Redirect(http.StatusSeeOther, "/onboarding")
	}
	if profile.HealthDataConsentAt != nil {
		return c.Redirect(http.StatusSeeOther, h.postLoginPath(user.ID))
	}

	if c.FormValue("health_data_consent") != "on" {
		return c.Render(http.StatusBadRequest, "consent.html", map[string]interface{}{
			"Error": "กรุณายินยอมให้เก็บและประมวลผลข้อมูลสุขภาพก่อนใช้งานต่อ",
		})
	}

	now := time.Now()
	version := HealthDataConsentVersion
	if err := h.DB.Model(&profile).Updates(map[string]interface{}{
		"health_data_consent_at":      now,
		"health_data_consent_version": version,
	}).Error; err != nil {
		log.Printf("consent: save consent failed for user_id=%d: %v", user.ID, err)
		return c.Render(http.StatusInternalServerError, "consent.html", map[string]interface{}{
			"Error": "เกิดข้อผิดพลาด กรุณาลองใหม่อีกครั้ง",
		})
	}

	return c.Redirect(http.StatusSeeOther, h.postLoginPath(user.ID))
}

// ---- POST /consent/withdraw — reachable from /profile's consent-status
// section (moved there from the account-menu dropdown so there's a single
// place to manage this, see _app_shell.html), gated by a JS confirm() the
// same way APD/CAPD entry deletion is. Does not touch previously-recorded
// health data, only blocks the log-book/food-check features going forward
// until the user consents again (see requireOnboardedUser/
// requireApdPatient/requireCapdPatient/requireLoggedInMember). ----

func (h *AuthHandler) WithdrawConsent(c echo.Context) error {
	user, err := h.currentSession(c)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/login")
	}

	if err := h.DB.Model(&models.PatientProfile{}).Where("user_id = ?", user.ID).Updates(map[string]interface{}{
		"health_data_consent_at":      nil,
		"health_data_consent_version": nil,
	}).Error; err != nil {
		log.Printf("consent: withdraw failed for user_id=%d: %v", user.ID, err)
		return c.Redirect(http.StatusSeeOther, "/profile?error=generic")
	}

	return c.Redirect(http.StatusSeeOther, "/profile?success=withdraw")
}
