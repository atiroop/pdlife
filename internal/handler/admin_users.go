package handler

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/atiroop/pdlife/internal/auth"
	"github.com/atiroop/pdlife/internal/models"
)

// Admin user management — account-level only, by hard rule. Nothing in
// this file may query or join apd_log_entries, capd_log_entries,
// hd_log_entries, lab_results, foodcheck_search_history, or any other
// health-data table: admins manage accounts here, never patient data.
// Every action against another user's account writes an admin_action_logs
// row in the same transaction as the action itself — an action can never
// happen unlogged.

const adminUsersPerPage = 20

// ---- GET /admin/users ----

func (h *AuthHandler) AdminUsersList(c echo.Context) error {
	admin, err := h.requireAdmin(c)
	if admin == nil {
		return err
	}
	nav := h.navInfoForUser(admin)

	q := strings.TrimSpace(c.QueryParam("q"))
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	query := h.DB.Model(&models.User{})
	if q != "" {
		like := "%" + q + "%"
		query = query.Where("email LIKE ? OR nickname LIKE ?", like, like)
	}

	var total int64
	query.Count(&total)
	totalPages := int((total + adminUsersPerPage - 1) / adminUsersPerPage)
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	var users []models.User
	query.Order("created_at DESC, id DESC").
		Limit(adminUsersPerPage).Offset((page - 1) * adminUsersPerPage).
		Find(&users)

	// treatment_type is the only patient_profiles column read here —
	// account-level context an admin legitimately needs; never anything
	// from the log-book/lab tables.
	userIDs := make([]uint64, len(users))
	for i, u := range users {
		userIDs[i] = u.ID
	}
	treatmentByUserID := map[uint64]string{}
	if len(userIDs) > 0 {
		var profiles []models.PatientProfile
		h.DB.Select("user_id", "treatment_type").Where("user_id IN ?", userIDs).Find(&profiles)
		for _, p := range profiles {
			if p.TreatmentType != nil {
				treatmentByUserID[p.UserID] = string(*p.TreatmentType)
			}
		}
	}

	rows := make([]map[string]interface{}, len(users))
	for i, u := range users {
		rows[i] = map[string]interface{}{
			"User":      u,
			"Treatment": treatmentByUserID[u.ID],
			"Status":    adminUserStatus(u),
		}
	}

	data := map[string]interface{}{
		"Rows":       rows,
		"Query":      q,
		"Page":       page,
		"TotalPages": totalPages,
		"Total":      total,
		"HasPrev":    page > 1,
		"HasNext":    page < totalPages,
		"PrevPage":   page - 1,
		"NextPage":   page + 1,
	}
	return c.Render(http.StatusOK, "admin_users.html", withNav(data, admin, nav, "/admin/users"))
}

// adminUserStatus reduces a user row to the one status label the admin
// pages show: suspended wins over everything, then verified/unverified.
func adminUserStatus(u models.User) string {
	switch {
	case u.SuspendedAt != nil:
		return "suspended"
	case u.Role == models.RoleUnverified:
		return "unverified"
	default:
		return "verified"
	}
}

// ---- GET /admin/users/:id ----

func (h *AuthHandler) AdminUserDetail(c echo.Context) error {
	admin, err := h.requireAdmin(c)
	if admin == nil {
		return err
	}
	nav := h.navInfoForUser(admin)

	target, err := h.adminTargetUser(c)
	if target == nil {
		return err
	}

	// Account-level profile context only (treatment/coverage/hospital) +
	// whether health-data consent exists as a bare boolean — per the hard
	// rule above, no health data itself is ever loaded here.
	var profile models.PatientProfile
	hasProfile := h.DB.Where("user_id = ?", target.ID).First(&profile).Error == nil

	var actionLogs []models.AdminActionLog
	h.DB.Where("target_user_id = ?", target.ID).Order("created_at DESC").Limit(50).Find(&actionLogs)

	// Resolve admin nicknames for the log display in one query.
	adminIDs := map[uint64]struct{}{}
	for _, l := range actionLogs {
		adminIDs[l.AdminID] = struct{}{}
	}
	adminNameByID := map[uint64]string{}
	if len(adminIDs) > 0 {
		ids := make([]uint64, 0, len(adminIDs))
		for id := range adminIDs {
			ids = append(ids, id)
		}
		var admins []models.User
		h.DB.Select("id", "nickname", "email").Where("id IN ?", ids).Find(&admins)
		for _, a := range admins {
			adminNameByID[a.ID] = a.Nickname + " (" + a.Email + ")"
		}
	}
	logRows := make([]map[string]interface{}, len(actionLogs))
	for i, l := range actionLogs {
		adminName := adminNameByID[l.AdminID]
		if adminName == "" {
			adminName = "(บัญชี admin ถูกลบไปแล้ว)"
		}
		logRows[i] = map[string]interface{}{
			"Log":       l,
			"AdminName": adminName,
			"Label":     adminActionLabel(l.Action),
			"When":      FormatDateThai(l.CreatedAt) + " " + l.CreatedAt.Format("15:04"),
		}
	}

	data := map[string]interface{}{
		"Target":      target,
		"Status":      adminUserStatus(*target),
		"HasProfile":  hasProfile,
		"IsLocked":    auth.LoginLimiter.IsLocked(target.Email),
		"ActionLogs":  logRows,
		"Success":     c.QueryParam("success"),
		"ErrorMsg":    adminUserErrorMessage(c.QueryParam("error")),
	}
	if hasProfile {
		treatment, coverage, hospital := "", "", ""
		if profile.TreatmentType != nil {
			treatment = string(*profile.TreatmentType)
		}
		if profile.CoverageType != nil {
			coverage = string(*profile.CoverageType)
		}
		if profile.HospitalName != nil {
			hospital = *profile.HospitalName
		}
		data["Treatment"] = treatment
		data["Coverage"] = coverage
		data["Hospital"] = hospital
		data["HasHealthConsent"] = profile.HealthDataConsentAt != nil
	}
	return c.Render(http.StatusOK, "admin_user_detail.html", withNav(data, admin, nav, "/admin/users"))
}

func adminActionLabel(a models.AdminAction) string {
	switch a {
	case models.AdminActionManualVerifyEmail:
		return "ยืนยันอีเมลแทน"
	case models.AdminActionUnlockAccount:
		return "ปลดล็อกบัญชี"
	case models.AdminActionSuspendAccount:
		return "ระงับบัญชี"
	case models.AdminActionUnsuspendAccount:
		return "ยกเลิกการระงับ"
	}
	return string(a)
}

func adminUserErrorMessage(code string) string {
	switch code {
	case "reason_required":
		return "ต้องกรอกเหตุผลก่อนระงับบัญชี"
	case "self_action":
		return "ไม่สามารถทำ action นี้กับบัญชีของตัวเองได้"
	case "invalid_state":
		return "สถานะบัญชีเปลี่ยนไปแล้ว กรุณาตรวจสอบอีกครั้ง"
	case "generic":
		return "เกิดข้อผิดพลาด กรุณาลองใหม่อีกครั้ง"
	}
	return ""
}

// adminTargetUser parses :id and loads the target user, rendering a 404
// placeholder when missing (nil user signals the response is written).
func (h *AuthHandler) adminTargetUser(c echo.Context) (*models.User, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return nil, c.Render(http.StatusNotFound, "placeholder.html", map[string]string{
			"Title":   "ไม่พบผู้ใช้",
			"Message": "ไม่พบผู้ใช้ที่ระบุ",
		})
	}
	var target models.User
	if err := h.DB.First(&target, id).Error; err != nil {
		return nil, c.Render(http.StatusNotFound, "placeholder.html", map[string]string{
			"Title":   "ไม่พบผู้ใช้",
			"Message": "ไม่พบผู้ใช้ที่ระบุ",
		})
	}
	return &target, nil
}

// logAdminAction appends the mandatory audit row inside tx — every action
// handler below calls this in the same transaction as its own writes.
func logAdminAction(tx *gorm.DB, adminID, targetID uint64, action models.AdminAction, reason *string) error {
	return tx.Create(&models.AdminActionLog{
		AdminID:      adminID,
		TargetUserID: targetID,
		Action:       action,
		Reason:       reason,
	}).Error
}

// ---- POST /admin/users/:id/verify-email ----

func (h *AuthHandler) AdminVerifyEmail(c echo.Context) error {
	admin, err := h.requireAdmin(c)
	if admin == nil {
		return err
	}
	target, err := h.adminTargetUser(c)
	if target == nil {
		return err
	}
	redirect := "/admin/users/" + strconv.FormatUint(target.ID, 10)

	if target.Role != models.RoleUnverified {
		return c.Redirect(http.StatusSeeOther, redirect+"?error=invalid_state")
	}

	now := time.Now()
	txErr := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(target).Updates(map[string]interface{}{
			"role":              models.RoleMember,
			"email_verified_at": now,
		}).Error; err != nil {
			return err
		}
		return logAdminAction(tx, admin.ID, target.ID, models.AdminActionManualVerifyEmail, nil)
	})
	if txErr != nil {
		log.Printf("admin-users: manual verify email failed (admin=%d target=%d): %v", admin.ID, target.ID, txErr)
		return c.Redirect(http.StatusSeeOther, redirect+"?error=generic")
	}
	log.Printf("admin-users: admin=%d manually verified email of user=%d", admin.ID, target.ID)
	return c.Redirect(http.StatusSeeOther, redirect+"?success=verified")
}

// ---- POST /admin/users/:id/unlock ----

func (h *AuthHandler) AdminUnlockAccount(c echo.Context) error {
	admin, err := h.requireAdmin(c)
	if admin == nil {
		return err
	}
	target, err := h.adminTargetUser(c)
	if target == nil {
		return err
	}
	redirect := "/admin/users/" + strconv.FormatUint(target.ID, 10)

	if !auth.LoginLimiter.IsLocked(target.Email) {
		return c.Redirect(http.StatusSeeOther, redirect+"?error=invalid_state")
	}

	// The lockout state lives in memory (auth.LoginLimiter), not the DB,
	// so the audit log row is the only transactional write here. Log
	// first: if the log write fails, no unlogged unlock happens.
	txErr := h.DB.Transaction(func(tx *gorm.DB) error {
		return logAdminAction(tx, admin.ID, target.ID, models.AdminActionUnlockAccount, nil)
	})
	if txErr != nil {
		log.Printf("admin-users: unlock account log failed (admin=%d target=%d): %v", admin.ID, target.ID, txErr)
		return c.Redirect(http.StatusSeeOther, redirect+"?error=generic")
	}
	auth.LoginLimiter.Reset(target.Email)
	log.Printf("admin-users: admin=%d unlocked login for user=%d", admin.ID, target.ID)
	return c.Redirect(http.StatusSeeOther, redirect+"?success=unlocked")
}

// ---- POST /admin/users/:id/suspend ----

func (h *AuthHandler) AdminSuspendAccount(c echo.Context) error {
	admin, err := h.requireAdmin(c)
	if admin == nil {
		return err
	}
	target, err := h.adminTargetUser(c)
	if target == nil {
		return err
	}
	redirect := "/admin/users/" + strconv.FormatUint(target.ID, 10)

	// An admin suspending their own account would lock the admin out of
	// the very page needed to undo it.
	if target.ID == admin.ID {
		return c.Redirect(http.StatusSeeOther, redirect+"?error=self_action")
	}
	if target.SuspendedAt != nil {
		return c.Redirect(http.StatusSeeOther, redirect+"?error=invalid_state")
	}
	reason := strings.TrimSpace(c.FormValue("reason"))
	if reason == "" {
		return c.Redirect(http.StatusSeeOther, redirect+"?error=reason_required")
	}

	// Rotating security_stamp invalidates every already-issued access JWT
	// on the next request, and revoking refresh tokens prevents getting a
	// new one — together this is the immediate forced logout the spec
	// requires, same mechanism as profile.go's password change.
	newStamp, err := auth.NewRandomToken()
	if err != nil {
		log.Printf("admin-users: generate stamp failed (admin=%d target=%d): %v", admin.ID, target.ID, err)
		return c.Redirect(http.StatusSeeOther, redirect+"?error=generic")
	}

	now := time.Now()
	txErr := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(target).Updates(map[string]interface{}{
			"suspended_at":     now,
			"suspended_reason": reason,
			"security_stamp":   newStamp,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.RefreshToken{}).
			Where("user_id = ? AND revoked_at IS NULL", target.ID).
			Update("revoked_at", now).Error; err != nil {
			return err
		}
		return logAdminAction(tx, admin.ID, target.ID, models.AdminActionSuspendAccount, &reason)
	})
	if txErr != nil {
		log.Printf("admin-users: suspend failed (admin=%d target=%d): %v", admin.ID, target.ID, txErr)
		return c.Redirect(http.StatusSeeOther, redirect+"?error=generic")
	}
	log.Printf("admin-users: admin=%d suspended user=%d", admin.ID, target.ID)
	return c.Redirect(http.StatusSeeOther, redirect+"?success=suspended")
}

// ---- POST /admin/users/:id/unsuspend ----

func (h *AuthHandler) AdminUnsuspendAccount(c echo.Context) error {
	admin, err := h.requireAdmin(c)
	if admin == nil {
		return err
	}
	target, err := h.adminTargetUser(c)
	if target == nil {
		return err
	}
	redirect := "/admin/users/" + strconv.FormatUint(target.ID, 10)

	if target.SuspendedAt == nil {
		return c.Redirect(http.StatusSeeOther, redirect+"?error=invalid_state")
	}

	txErr := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(target).Updates(map[string]interface{}{
			"suspended_at":     nil,
			"suspended_reason": nil,
		}).Error; err != nil {
			return err
		}
		return logAdminAction(tx, admin.ID, target.ID, models.AdminActionUnsuspendAccount, nil)
	})
	if txErr != nil {
		log.Printf("admin-users: unsuspend failed (admin=%d target=%d): %v", admin.ID, target.ID, txErr)
		return c.Redirect(http.StatusSeeOther, redirect+"?error=generic")
	}
	log.Printf("admin-users: admin=%d unsuspended user=%d", admin.ID, target.ID)
	return c.Redirect(http.StatusSeeOther, redirect+"?success=unsuspended")
}
