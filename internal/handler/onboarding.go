package handler

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

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

func sessionExpiredPage(c echo.Context) error {
	return c.Render(http.StatusUnauthorized, "placeholder.html", map[string]string{
		"Title":   "เซสชันหมดอายุ",
		"Message": "กรุณาเข้าสู่ระบบอีกครั้งเพื่อทำ onboarding ต่อ",
	})
}

func (h *AuthHandler) OnboardingForm(c echo.Context) error {
	user, err := h.currentSession(c)
	if err != nil {
		return sessionExpiredPage(c)
	}

	if h.hasCompletedOnboarding(user.ID) {
		return c.Render(http.StatusOK, "placeholder.html", map[string]string{
			"Title":   "ทำ onboarding ครบแล้ว",
			"Message": "คุณกรอกข้อมูลผู้ป่วยครบถ้วนแล้ว",
		})
	}

	return c.Render(http.StatusOK, "onboarding.html", map[string]interface{}{"Error": ""})
}

func (h *AuthHandler) OnboardingSubmit(c echo.Context) error {
	user, err := h.currentSession(c)
	if err != nil {
		return sessionExpiredPage(c)
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
	err = h.DB.Where("user_id = ?", user.ID).First(&profile).Error
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
			UserID:             user.ID,
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
