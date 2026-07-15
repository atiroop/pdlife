package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/models"
)

type hdLogInput struct {
	LogDate                 time.Time
	DryWeightKG             float64
	PreDialysisWeightKG     float64
	PostDialysisWeightKG    float64
	PreDialysisBPSystolic   int
	PreDialysisBPDiastolic  int
	PostDialysisBPSystolic  int
	PostDialysisBPDiastolic int
	Notes                   *string
}

func parseHdLogInput(c echo.Context) (*hdLogInput, error) {
	dateStr, err := requiredString(c, "logDate", "วันที่")
	if err != nil {
		return nil, err
	}
	logDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil, &formValidationError{"รูปแบบวันที่ไม่ถูกต้อง"}
	}

	var l hdLogInput
	l.LogDate = logDate

	if l.DryWeightKG, err = requiredFloat(c, "dryWeightKg", "น้ำหนักแห้ง"); err != nil {
		return nil, err
	}
	if l.PreDialysisWeightKG, err = requiredFloat(c, "preDialysisWeightKg", "น้ำหนักก่อนฟอก"); err != nil {
		return nil, err
	}
	if l.PostDialysisWeightKG, err = requiredFloat(c, "postDialysisWeightKg", "น้ำหนักหลังฟอก"); err != nil {
		return nil, err
	}
	if l.PreDialysisBPSystolic, err = requiredInt(c, "preDialysisBpSystolic", "ความดันก่อนฟอกตัวบน"); err != nil {
		return nil, err
	}
	if l.PreDialysisBPDiastolic, err = requiredInt(c, "preDialysisBpDiastolic", "ความดันก่อนฟอกตัวล่าง"); err != nil {
		return nil, err
	}
	if l.PostDialysisBPSystolic, err = requiredInt(c, "postDialysisBpSystolic", "ความดันหลังฟอกตัวบน"); err != nil {
		return nil, err
	}
	if l.PostDialysisBPDiastolic, err = requiredInt(c, "postDialysisBpDiastolic", "ความดันหลังฟอกตัวล่าง"); err != nil {
		return nil, err
	}
	l.Notes = optionalString(c, "notes")

	return &l, nil
}

func hdFormData(entry *models.HdLogEntry, formErr string) map[string]interface{} {
	return map[string]interface{}{
		"Log":   entry,
		"Error": formErr,
		"Today": time.Now().Format("2006-01-02"),
	}
}

// ---- GET /hd/new ----

func (h *AuthHandler) HdNewForm(c echo.Context) error {
	user, profile, err := h.requireHdPatient(c)
	if user == nil {
		return err
	}
	data := hdFormData(nil, "")
	data["IsEditing"] = false
	return c.Render(http.StatusOK, "hd_form.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/hd/new"))
}

// ---- POST /hd/new ----

func (h *AuthHandler) HdCreate(c echo.Context) error {
	user, profile, err := h.requireHdPatient(c)
	if user == nil {
		return err
	}

	renderErr := func(msg string) error {
		data := hdFormData(nil, msg)
		data["IsEditing"] = false
		return c.Render(http.StatusBadRequest, "hd_form.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/hd/new"))
	}

	logIn, err := parseHdLogInput(c)
	if err != nil {
		var verr *formValidationError
		if errors.As(err, &verr) {
			return renderErr(verr.msg)
		}
		return err
	}

	entry := models.HdLogEntry{
		PatientProfileID:        profile.ID,
		LogDate:                 logIn.LogDate,
		DryWeightKG:             logIn.DryWeightKG,
		PreDialysisWeightKG:     logIn.PreDialysisWeightKG,
		PostDialysisWeightKG:    logIn.PostDialysisWeightKG,
		PreDialysisBPSystolic:   logIn.PreDialysisBPSystolic,
		PreDialysisBPDiastolic:  logIn.PreDialysisBPDiastolic,
		PostDialysisBPSystolic:  logIn.PostDialysisBPSystolic,
		PostDialysisBPDiastolic: logIn.PostDialysisBPDiastolic,
		Notes:                   logIn.Notes,
	}
	entry.ComputeUFRemoved()

	if err := h.DB.Create(&entry).Error; err != nil {
		if isDuplicateEntryError(err) {
			return renderErr("มีบันทึกของวันที่นี้อยู่แล้ว")
		}
		return err
	}

	return c.Redirect(http.StatusSeeOther, "/hd")
}

// ---- GET /hd/:id/edit ----

func (h *AuthHandler) HdEditForm(c echo.Context) error {
	user, profile, err := h.requireHdPatient(c)
	if user == nil {
		return err
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/hd/logs")
	}

	var entry models.HdLogEntry
	if err := h.DB.Where("id = ? AND patient_profile_id = ?", id, profile.ID).First(&entry).Error; err != nil {
		return c.Redirect(http.StatusSeeOther, "/hd/logs")
	}

	data := hdFormData(&entry, "")
	data["IsEditing"] = true
	return c.Render(http.StatusOK, "hd_form.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/hd/logs"))
}

// ---- POST /hd/:id/edit ----

func (h *AuthHandler) HdUpdate(c echo.Context) error {
	user, profile, err := h.requireHdPatient(c)
	if user == nil {
		return err
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/hd/logs")
	}

	var existing models.HdLogEntry
	if err := h.DB.Where("id = ? AND patient_profile_id = ?", id, profile.ID).First(&existing).Error; err != nil {
		return c.Redirect(http.StatusSeeOther, "/hd/logs")
	}

	renderErr := func(msg string) error {
		data := hdFormData(&existing, msg)
		data["IsEditing"] = true
		return c.Render(http.StatusBadRequest, "hd_form.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/hd/logs"))
	}

	logIn, err := parseHdLogInput(c)
	if err != nil {
		var verr *formValidationError
		if errors.As(err, &verr) {
			return renderErr(verr.msg)
		}
		return err
	}

	existing.LogDate = logIn.LogDate
	existing.DryWeightKG = logIn.DryWeightKG
	existing.PreDialysisWeightKG = logIn.PreDialysisWeightKG
	existing.PostDialysisWeightKG = logIn.PostDialysisWeightKG
	existing.PreDialysisBPSystolic = logIn.PreDialysisBPSystolic
	existing.PreDialysisBPDiastolic = logIn.PreDialysisBPDiastolic
	existing.PostDialysisBPSystolic = logIn.PostDialysisBPSystolic
	existing.PostDialysisBPDiastolic = logIn.PostDialysisBPDiastolic
	existing.Notes = logIn.Notes
	existing.ComputeUFRemoved()

	if err := h.DB.Save(&existing).Error; err != nil {
		if isDuplicateEntryError(err) {
			return renderErr("มีบันทึกของวันที่นี้อยู่แล้ว")
		}
		return err
	}

	return c.Redirect(http.StatusSeeOther, "/hd/logs")
}

// ---- POST /hd/:id/delete ----

func (h *AuthHandler) HdDelete(c echo.Context) error {
	user, profile, err := h.requireHdPatient(c)
	if user == nil {
		return err
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/hd/logs")
	}

	var entry models.HdLogEntry
	if err := h.DB.Where("id = ? AND patient_profile_id = ?", id, profile.ID).First(&entry).Error; err != nil {
		return c.Redirect(http.StatusSeeOther, "/hd/logs")
	}

	if err := h.DB.Delete(&entry).Error; err != nil {
		return err
	}

	_ = user
	return c.Redirect(http.StatusSeeOther, "/hd/logs")
}
