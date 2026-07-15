// Package handler: the "จัดการโปรไฟล์" (/profile) account-management
// page — edit name, change password, edit treatment info, health-data
// consent status/withdrawal (moved here from the account-menu dropdown,
// see _app_shell.html), and PDPA data-subject rights (export, deletion
// request). See cmd/purge_deleted_accounts for what happens 90 days
// after a deletion request.
package handler

import (
	"encoding/json"
	"fmt"
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

// AccountDeletionGraceDays is how long a deletion request sits before
// cmd/purge_deleted_accounts hard-deletes/anonymizes the account — keep
// this in sync with that command's own purge-window constant and with the
// wording in internal/mailer/templates/account_deletion.{html,txt}.
const AccountDeletionGraceDays = 90

// profileSupportEmail matches the PDPA contact already published in
// /privacy (§1) — see internal/handler/consent.go's LegalContentUpdatedDate
// for the sibling "keep these in sync" constant.
const profileSupportEmail = "support@pdlife.app"

const deleteAccountConfirmPhrase = "ลบบัญชี"

// ---- GET /profile ----

func (h *AuthHandler) ProfileForm(c echo.Context) error {
	user, profile, err := h.requireOnboardedUser(c)
	if user == nil {
		return err
	}
	nav := h.navInfoFromProfile(user, profile)

	data := h.profilePageData(profile, c.QueryParam("success"), c.QueryParam("error"))
	return c.Render(http.StatusOK, "profile.html", withNav(data, user, nav, "/profile"))
}

// profilePageData translates the redirect-back ?success=/?error= query
// codes every POST handler below uses into the Thai banner text profile.html
// shows — kept as one map so every POST handler's redirect target renders
// consistently without each handler re-deriving message text itself.
func (h *AuthHandler) profilePageData(profile *models.PatientProfile, successCode, errorCode string) map[string]interface{} {
	successMessages := map[string]string{
		"name":      "บันทึกชื่อเรียบร้อยแล้ว",
		"password":  "เปลี่ยนรหัสผ่านเรียบร้อยแล้ว อุปกรณ์อื่นที่เคยเข้าสู่ระบบไว้จะต้องเข้าสู่ระบบใหม่",
		"treatment": "บันทึกข้อมูลการรักษาเรียบร้อยแล้ว",
		"withdraw":  "ถอนความยินยอมข้อมูลสุขภาพเรียบร้อยแล้ว",
	}
	errorMessages := map[string]string{
		"name_empty":              "กรุณากรอกชื่อ",
		"password_current_wrong":  "รหัสผ่านปัจจุบันไม่ถูกต้อง",
		"password_mismatch":       "รหัสผ่านใหม่และการยืนยันไม่ตรงกัน",
		"treatment_invalid":       "กรุณาเลือกวิธีการรักษา",
		"coverage_invalid":        "กรุณาเลือกสิทธิการรักษา",
		"hospital_empty":          "กรุณากรอกชื่อโรงพยาบาล",
		"delete_password_wrong":   "รหัสผ่านไม่ถูกต้อง",
		"delete_confirm_mismatch": fmt.Sprintf("กรุณาพิมพ์คำว่า %q ให้ตรงเพื่อยืนยัน", deleteAccountConfirmPhrase),
		"generic":                 "เกิดข้อผิดพลาด กรุณาลองใหม่อีกครั้ง",
	}

	success := successMessages[successCode]
	errMsg := errorMessages[errorCode]
	if errMsg == "" && errorCode != "" {
		// auth.ValidatePasswordStrength's message is dynamic (mentions the
		// exact rule broken) — passed straight through as the error code
		// itself rather than a fixed lookup key.
		errMsg = errorCode
	}

	// Plain (non-pointer) strings for the template — the "deref" template
	// func only special-cases *int/*string/*time.Time, not the named
	// TreatmentType/CoverageType string types, so dereferencing those in
	// Go here avoids template-side type gymnastics.
	treatmentTypeStr, coverageTypeStr, hospitalNameStr := "", "", ""
	if profile.TreatmentType != nil {
		treatmentTypeStr = string(*profile.TreatmentType)
	}
	if profile.CoverageType != nil {
		coverageTypeStr = string(*profile.CoverageType)
	}
	if profile.HospitalName != nil {
		hospitalNameStr = *profile.HospitalName
	}
	consentAtStr, consentVersionStr := "", ""
	if profile.HealthDataConsentAt != nil {
		consentAtStr = FormatDateThai(*profile.HealthDataConsentAt)
	}
	if profile.HealthDataConsentVersion != nil {
		consentVersionStr = *profile.HealthDataConsentVersion
	}

	return map[string]interface{}{
		"Success":                    success,
		"Error":                      errMsg,
		"DeleteAccountConfirmPhrase": deleteAccountConfirmPhrase,
		"TreatmentType":              treatmentTypeStr,
		"CoverageType":               coverageTypeStr,
		"HospitalName":               hospitalNameStr,
		"HasConsent":                 profile.HealthDataConsentAt != nil,
		"ConsentAt":                  consentAtStr,
		"ConsentVersion":             consentVersionStr,
	}
}

// ---- POST /profile/name ----

func (h *AuthHandler) ProfileUpdateName(c echo.Context) error {
	user, _, err := h.requireOnboardedUser(c)
	if user == nil {
		return err
	}

	nickname := strings.TrimSpace(c.FormValue("nickname"))
	if nickname == "" {
		return c.Redirect(http.StatusSeeOther, "/profile?error=name_empty")
	}
	if err := h.DB.Model(user).Update("nickname", nickname).Error; err != nil {
		log.Printf("profile: update nickname failed for user_id=%d: %v", user.ID, err)
		return c.Redirect(http.StatusSeeOther, "/profile?error=generic")
	}
	return c.Redirect(http.StatusSeeOther, "/profile?success=name")
}

// ---- POST /profile/password ----

func (h *AuthHandler) ProfileChangePassword(c echo.Context) error {
	user, _, err := h.requireOnboardedUser(c)
	if user == nil {
		return err
	}

	current := c.FormValue("current_password")
	newPassword := c.FormValue("new_password")
	confirm := c.FormValue("confirm_password")

	if !auth.CheckPassword(user.PasswordHash, current) {
		return c.Redirect(http.StatusSeeOther, "/profile?error=password_current_wrong")
	}
	if newPassword != confirm {
		return c.Redirect(http.StatusSeeOther, "/profile?error=password_mismatch")
	}
	if verr := auth.ValidatePasswordStrength(newPassword); verr != nil {
		return c.Redirect(http.StatusSeeOther, "/profile?error="+verr.Error())
	}

	passwordHash, err := auth.HashPassword(newPassword)
	if err != nil {
		log.Printf("profile: hash password failed for user_id=%d: %v", user.ID, err)
		return c.Redirect(http.StatusSeeOther, "/profile?error=generic")
	}
	newStamp, err := auth.NewRandomToken()
	if err != nil {
		log.Printf("profile: generate stamp failed for user_id=%d: %v", user.ID, err)
		return c.Redirect(http.StatusSeeOther, "/profile?error=generic")
	}

	now := time.Now()
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(user).Updates(map[string]interface{}{
			"password_hash":  passwordHash,
			"security_stamp": newStamp, // invalidates every previously issued JWT at once
		}).Error; err != nil {
			return err
		}
		// Revoke every outstanding refresh token — forces re-login on
		// every OTHER device. The current device is handled below by
		// re-issuing fresh cookies with the new stamp, so it stays
		// logged in (the user just proved they know the password).
		return tx.Model(&models.RefreshToken{}).
			Where("user_id = ? AND revoked_at IS NULL", user.ID).
			Update("revoked_at", now).Error
	}); err != nil {
		log.Printf("profile: change password failed for user_id=%d: %v", user.ID, err)
		return c.Redirect(http.StatusSeeOther, "/profile?error=generic")
	}

	user.PasswordHash = passwordHash
	user.SecurityStamp = newStamp
	if err := h.issueSessionCookies(c, *user); err != nil {
		log.Printf("profile: reissue session after password change failed for user_id=%d: %v", user.ID, err)
	}

	return c.Redirect(http.StatusSeeOther, "/profile?success=password")
}

// ---- POST /profile/treatment ----

func (h *AuthHandler) ProfileUpdateTreatment(c echo.Context) error {
	user, profile, err := h.requireOnboardedUser(c)
	if user == nil {
		return err
	}

	treatmentInput := c.FormValue("treatment_type")
	coverageInput := c.FormValue("coverage_type")
	hospitalName := strings.TrimSpace(c.FormValue("hospital_name"))

	treatment, ok := validTreatmentTypes[treatmentInput]
	if !ok {
		return c.Redirect(http.StatusSeeOther, "/profile?error=treatment_invalid")
	}
	coverage, ok := validCoverageTypes[coverageInput]
	if !ok {
		return c.Redirect(http.StatusSeeOther, "/profile?error=coverage_invalid")
	}
	if hospitalName == "" {
		return c.Redirect(http.StatusSeeOther, "/profile?error=hospital_empty")
	}

	// Changing treatment_type only ever changes which menu/gate a patient
	// sees — existing apd_log_entries/capd_log_entries rows are never
	// touched, they simply stop being reachable from the main nav until
	// the patient switches back (see requireApdPatient/requireCapdPatient).
	// The "are you sure" prompt for this specific field lives client-side
	// in profile.html (dynamic message per old→new combination) — by the
	// time this handler runs, that confirmation has already happened.
	if err := h.DB.Model(profile).Updates(map[string]interface{}{
		"treatment_type": treatment,
		"coverage_type":  coverage,
		"hospital_name":  hospitalName,
	}).Error; err != nil {
		log.Printf("profile: update treatment info failed for user_id=%d: %v", user.ID, err)
		return c.Redirect(http.StatusSeeOther, "/profile?error=generic")
	}

	return c.Redirect(http.StatusSeeOther, "/profile?success=treatment")
}

// ---- GET /profile/export-data ----

// profileExport is a plain, display-friendly JSON shape (not the GORM
// models directly re-marshaled unfiltered) so PasswordHash/SecurityStamp
// never end up in the file even by accident from a future model field
// addition — every field here is named explicitly.
type profileExportAccount struct {
	ID              uint64     `json:"id"`
	Email           string     `json:"email"`
	Nickname        string     `json:"nickname"`
	Role            string     `json:"role"`
	IsActive        bool       `json:"is_active"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
	LastLoginAt     *time.Time `json:"last_login_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (h *AuthHandler) ProfileExportData(c echo.Context) error {
	user, profile, err := h.requireOnboardedUser(c)
	if user == nil {
		return err
	}

	export := map[string]interface{}{
		"exported_at": time.Now().Format(time.RFC3339),
		"account": profileExportAccount{
			ID:              user.ID,
			Email:           user.Email,
			Nickname:        user.Nickname,
			Role:            string(user.Role),
			IsActive:        user.IsActive,
			EmailVerifiedAt: user.EmailVerifiedAt,
			LastLoginAt:     user.LastLoginAt,
			CreatedAt:       user.CreatedAt,
			UpdatedAt:       user.UpdatedAt,
		},
		"patient_profile": profile,
	}

	if profile.TreatmentType != nil {
		switch *profile.TreatmentType {
		case models.TreatmentAPD:
			var entries []models.ApdLogEntry
			h.DB.Where("patient_profile_id = ?", profile.ID).Order("entry_date").Find(&entries)
			export["apd_log_entries"] = entries
			var prescriptions []models.ApdPrescription
			h.DB.Where("patient_profile_id = ?", profile.ID).Find(&prescriptions)
			export["apd_prescriptions"] = prescriptions
		case models.TreatmentCAPD:
			var entries []models.CapdLogEntry
			h.DB.Where("patient_profile_id = ?", profile.ID).Order("log_date, cycle_number").Find(&entries)
			export["capd_log_entries"] = entries
		}
	}

	var searchHistory []models.FoodCheckSearchHistory
	h.DB.Where("patient_profile_id = ?", profile.ID).Find(&searchHistory)
	export["food_search_history"] = searchHistory

	var articles []models.EditorialArticle
	h.DB.Where("author_id = ?", user.ID).Find(&articles)
	if len(articles) > 0 {
		export["editorial_articles"] = articles
	}

	jsonBytes, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		log.Printf("profile: export marshal failed for user_id=%d: %v", user.ID, err)
		return c.String(http.StatusInternalServerError, "เกิดข้อผิดพลาด กรุณาลองใหม่อีกครั้ง")
	}

	filename := fmt.Sprintf("pdlife-my-data-%s.json", time.Now().Format("20060102"))
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	return c.Blob(http.StatusOK, "application/json", jsonBytes)
}

// ---- POST /profile/delete-account ----

func (h *AuthHandler) ProfileDeleteAccount(c echo.Context) error {
	user, _, err := h.requireOnboardedUser(c)
	if user == nil {
		return err
	}

	password := c.FormValue("password")
	confirmText := strings.TrimSpace(c.FormValue("confirm_text"))

	if !auth.CheckPassword(user.PasswordHash, password) {
		return c.Redirect(http.StatusSeeOther, "/profile?error=delete_password_wrong")
	}
	if confirmText != deleteAccountConfirmPhrase {
		return c.Redirect(http.StatusSeeOther, "/profile?error=delete_confirm_mismatch")
	}

	now := time.Now()
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(user).Update("account_deletion_requested_at", now).Error; err != nil {
			return err
		}
		return tx.Model(&models.RefreshToken{}).
			Where("user_id = ? AND revoked_at IS NULL", user.ID).
			Update("revoked_at", now).Error
	}); err != nil {
		log.Printf("profile: delete-account request failed for user_id=%d: %v", user.ID, err)
		return c.Redirect(http.StatusSeeOther, "/profile?error=generic")
	}

	purgeDate := now.AddDate(0, 0, AccountDeletionGraceDays)
	if err := h.Mailer.SendAccountDeletionEmail(user.Email, mailer.AccountDeletionData{
		Nickname:     user.Nickname,
		Email:        user.Email,
		PurgeDate:    FormatDateThai(purgeDate),
		SupportEmail: profileSupportEmail,
	}); err != nil {
		log.Printf("profile: send deletion email failed for user_id=%d: %v", user.ID, err)
	}

	h.clearSessionCookies(c)
	return c.Render(http.StatusOK, "account_deletion_requested.html", map[string]interface{}{
		"PurgeDate": FormatDateThai(purgeDate),
	})
}
