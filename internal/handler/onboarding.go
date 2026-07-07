package handler

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/atiroop/pdlife/internal/auth"
	"github.com/atiroop/pdlife/internal/models"
)

var validTreatmentTypes = map[string]models.TreatmentType{
	"CAPD": models.TreatmentCAPD,
	"APD":  models.TreatmentAPD,
	"HD":   models.TreatmentHD,
}

var validCoverageTypes = map[string]models.CoverageType{
	string(models.CoverageGoldCard):     models.CoverageGoldCard,
	string(models.CoverageSocialSecure): models.CoverageSocialSecure,
	string(models.CoverageCivilServant): models.CoverageCivilServant,
	string(models.CoverageOther):        models.CoverageOther,
}

// currentSession reads and verifies the pdlife_session cookie set right
// after email verification. There's no full login/refresh flow yet (a
// separate follow-up task), so this cookie is the only way in.
func (h *AuthHandler) currentSession(c echo.Context) (*auth.SessionClaims, error) {
	cookie, err := c.Cookie(auth.SessionCookieName)
	if err != nil {
		return nil, err
	}
	return auth.ParseSessionToken(h.Cfg.JWTSecret, cookie.Value)
}

func (h *AuthHandler) OnboardingForm(c echo.Context) error {
	claims, err := h.currentSession(c)
	if err != nil {
		return c.Render(http.StatusUnauthorized, "placeholder.html", map[string]string{
			"Title":   "เซสชันหมดอายุ",
			"Message": "กรุณายืนยันอีเมลอีกครั้งเพื่อเริ่ม onboarding",
		})
	}

	var profile models.PatientProfile
	err = h.DB.Where("user_id = ?", claims.UserID).First(&profile).Error
	if err == nil && profile.ProfileCompletedAt != nil {
		return c.Render(http.StatusOK, "placeholder.html", map[string]string{
			"Title":   "ทำ onboarding ครบแล้ว",
			"Message": "คุณกรอกข้อมูลผู้ป่วยครบถ้วนแล้ว",
		})
	}

	return c.Render(http.StatusOK, "onboarding.html", map[string]interface{}{"Error": ""})
}

func (h *AuthHandler) OnboardingSubmit(c echo.Context) error {
	claims, err := h.currentSession(c)
	if err != nil {
		return c.Render(http.StatusUnauthorized, "placeholder.html", map[string]string{
			"Title":   "เซสชันหมดอายุ",
			"Message": "กรุณายืนยันอีเมลอีกครั้งเพื่อเริ่ม onboarding",
		})
	}

	treatmentInput := c.FormValue("treatment_type")
	coverageInput := c.FormValue("coverage_type")
	hospitalName := strings.TrimSpace(c.FormValue("hospital_name"))

	treatment, ok := validTreatmentTypes[treatmentInput]
	if !ok {
		return c.Render(http.StatusBadRequest, "onboarding.html", map[string]interface{}{
			"Error": "กรุณาเลือกวิธีการรักษา",
		})
	}
	coverage, ok := validCoverageTypes[coverageInput]
	if !ok {
		return c.Render(http.StatusBadRequest, "onboarding.html", map[string]interface{}{
			"Error": "กรุณาเลือกสิทธิการรักษา",
		})
	}
	if hospitalName == "" {
		return c.Render(http.StatusBadRequest, "onboarding.html", map[string]interface{}{
			"Error": "กรุณากรอกชื่อโรงพยาบาล",
		})
	}

	now := time.Now()
	var profile models.PatientProfile
	err = h.DB.Where("user_id = ?", claims.UserID).First(&profile).Error
	switch {
	case err == nil:
		err = h.DB.Model(&profile).Updates(map[string]interface{}{
			"treatment_type":       treatment,
			"hospital_name":        hospitalName,
			"coverage_type":        coverage,
			"profile_completed_at": now,
		}).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		profile = models.PatientProfile{
			UserID:             claims.UserID,
			TreatmentType:      &treatment,
			HospitalName:       &hospitalName,
			CoverageType:       &coverage,
			ProfileCompletedAt: &now,
		}
		err = h.DB.Create(&profile).Error
	}
	if err != nil {
		log.Printf("onboarding: save profile failed: %v", err)
		return c.Render(http.StatusInternalServerError, "onboarding.html", map[string]interface{}{
			"Error": "เกิดข้อผิดพลาด กรุณาลองใหม่อีกครั้ง",
		})
	}

	return c.Render(http.StatusOK, "onboarding_complete.html", map[string]interface{}{
		"IsHD": treatment == models.TreatmentHD,
	})
}
