package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/models"
)

// dextroseConcentrationPresets mirrors the fixed dropdown options from the
// spec; "other" lets the patient type a value not in this list.
var dextroseConcentrationPresets = []string{"1.5", "2.5", "4.25"}

// dialysateAppearanceOptions is deliberately ordered clear-first but the
// form has NO default selection — the option list is rendered under a
// blank placeholder that fails validation if left chosen, forcing the
// patient to choose explicitly every time.
var dialysateAppearanceOptions = []models.DialysateAppearance{
	models.DialysateClear, models.DialysateCloudy, models.DialysateBloody,
}

type capdLogInput struct {
	LogDate               time.Time
	CycleNumber           int
	DextroseConcentration float64
	FillStartTime         string
	FillEndTime           string
	FillVolumeML          int
	DrainStartTime        string
	DrainEndTime          string
	DrainVolumeML         int
	DialysateAppearance   models.DialysateAppearance
	WeightKG              float64
	BPSystolic            int
	BPDiastolic           int
	UrineOutputML         *int
}

func parseCapdLogInput(c echo.Context) (*capdLogInput, error) {
	dateStr, err := requiredString(c, "logDate", "วันที่")
	if err != nil {
		return nil, err
	}
	logDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil, &formValidationError{"รูปแบบวันที่ไม่ถูกต้อง"}
	}

	var l capdLogInput
	l.LogDate = logDate

	if l.CycleNumber, err = requiredInt(c, "cycleNumber", "รอบที่"); err != nil {
		return nil, err
	}
	if l.CycleNumber < 1 || l.CycleNumber > 5 {
		return nil, &formValidationError{"รอบที่ต้องอยู่ระหว่าง 1-5"}
	}

	dextroseChoice, err := requiredString(c, "dextroseConcentration", "ความเข้มข้นเดกซ์โทรส")
	if err != nil {
		return nil, err
	}
	if dextroseChoice == "other" {
		other, err := requiredFloat(c, "dextroseConcentrationOther", "ความเข้มข้นเดกซ์โทรส")
		if err != nil {
			return nil, err
		}
		l.DextroseConcentration = other
	} else {
		v, err := strconv.ParseFloat(dextroseChoice, 64)
		if err != nil {
			return nil, &formValidationError{"ความเข้มข้นเดกซ์โทรสไม่ถูกต้อง"}
		}
		l.DextroseConcentration = v
	}

	if l.FillStartTime, err = requiredString(c, "fillStartTime", "เวลาเริ่มเติมน้ำยา"); err != nil {
		return nil, err
	}
	if l.FillEndTime, err = requiredString(c, "fillEndTime", "เวลาเติมน้ำยาเสร็จ"); err != nil {
		return nil, err
	}
	if l.FillVolumeML, err = requiredInt(c, "fillVolumeMl", "ปริมาตรเติมเข้า"); err != nil {
		return nil, err
	}
	if l.DrainStartTime, err = requiredString(c, "drainStartTime", "เวลาเริ่มปล่อยน้ำยาออก"); err != nil {
		return nil, err
	}
	if l.DrainEndTime, err = requiredString(c, "drainEndTime", "เวลาปล่อยน้ำยาออกเสร็จ"); err != nil {
		return nil, err
	}
	if l.DrainVolumeML, err = requiredInt(c, "drainVolumeMl", "ปริมาตรปล่อยออก"); err != nil {
		return nil, err
	}

	appearanceStr, err := requiredString(c, "dialysateAppearance", "ลักษณะน้ำยา")
	if err != nil {
		return nil, err
	}
	valid := false
	for _, opt := range dialysateAppearanceOptions {
		if string(opt) == appearanceStr {
			l.DialysateAppearance = opt
			valid = true
			break
		}
	}
	if !valid {
		return nil, &formValidationError{"กรุณาเลือกลักษณะน้ำยาที่ถูกต้อง"}
	}

	if l.WeightKG, err = requiredFloat(c, "weightKg", "น้ำหนัก"); err != nil {
		return nil, err
	}
	if l.BPSystolic, err = requiredInt(c, "bpSystolic", "ความดันตัวบน"); err != nil {
		return nil, err
	}
	if l.BPDiastolic, err = requiredInt(c, "bpDiastolic", "ความดันตัวล่าง"); err != nil {
		return nil, err
	}
	l.UrineOutputML = optionalInt(c, "urineOutputMl")

	return &l, nil
}

// matchDextrosePreset reports which dropdown option an existing entry's
// concentration corresponds to: one of dextroseConcentrationPresets, or
// "other" with the exact value to prefill the free-text field.
func matchDextrosePreset(v float64) (choice string, otherValue string) {
	for _, p := range dextroseConcentrationPresets {
		pf, _ := strconv.ParseFloat(p, 64)
		if pf == v {
			return p, ""
		}
	}
	return "other", strconv.FormatFloat(v, 'f', 2, 64)
}

func capdFormData(entry *models.CapdLogEntry, formErr string) map[string]interface{} {
	dextroseChoice := dextroseConcentrationPresets[0]
	dextroseOtherValue := ""
	if entry != nil {
		dextroseChoice, dextroseOtherValue = matchDextrosePreset(entry.DextroseConcentration)
	}
	return map[string]interface{}{
		"Log":                          entry,
		"Error":                        formErr,
		"Today":                        time.Now().Format("2006-01-02"),
		"DextroseConcentrationPresets": dextroseConcentrationPresets,
		"DialysateAppearanceOptions":   dialysateAppearanceOptions,
		"DextroseChoice":               dextroseChoice,
		"DextroseOtherValue":           dextroseOtherValue,
	}
}

// ---- GET /capd/new ----

func (h *AuthHandler) CapdNewForm(c echo.Context) error {
	user, profile, err := h.requireCapdPatient(c)
	if user == nil {
		return err
	}
	data := capdFormData(nil, "")
	data["IsEditing"] = false
	return c.Render(http.StatusOK, "capd_form.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/capd/new"))
}

// ---- POST /capd/new ----

func (h *AuthHandler) CapdCreate(c echo.Context) error {
	user, profile, err := h.requireCapdPatient(c)
	if user == nil {
		return err
	}

	renderErr := func(msg string) error {
		data := capdFormData(nil, msg)
		data["IsEditing"] = false
		return c.Render(http.StatusBadRequest, "capd_form.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/capd/new"))
	}

	logIn, err := parseCapdLogInput(c)
	if err != nil {
		var verr *formValidationError
		if errors.As(err, &verr) {
			return renderErr(verr.msg)
		}
		return err
	}

	entry := models.CapdLogEntry{
		PatientProfileID:      profile.ID,
		LogDate:               logIn.LogDate,
		CycleNumber:           logIn.CycleNumber,
		DextroseConcentration: logIn.DextroseConcentration,
		FillStartTime:         logIn.FillStartTime,
		FillEndTime:           logIn.FillEndTime,
		FillVolumeML:          logIn.FillVolumeML,
		DrainStartTime:        logIn.DrainStartTime,
		DrainEndTime:          logIn.DrainEndTime,
		DrainVolumeML:         logIn.DrainVolumeML,
		DialysateAppearance:   logIn.DialysateAppearance,
		WeightKG:              logIn.WeightKG,
		BPSystolic:            logIn.BPSystolic,
		BPDiastolic:           logIn.BPDiastolic,
		UrineOutputML:         logIn.UrineOutputML,
	}
	entry.ComputeUF()

	if err := h.DB.Create(&entry).Error; err != nil {
		if isDuplicateEntryError(err) {
			return renderErr("มีบันทึกของวันที่และรอบนี้อยู่แล้ว")
		}
		return err
	}

	return c.Redirect(http.StatusSeeOther, "/capd")
}

// ---- GET /capd/:id/edit ----

func (h *AuthHandler) CapdEditForm(c echo.Context) error {
	user, profile, err := h.requireCapdPatient(c)
	if user == nil {
		return err
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/capd/logs")
	}

	var entry models.CapdLogEntry
	if err := h.DB.Where("id = ? AND patient_profile_id = ?", id, profile.ID).First(&entry).Error; err != nil {
		return c.Redirect(http.StatusSeeOther, "/capd/logs")
	}

	data := capdFormData(&entry, "")
	data["IsEditing"] = true
	return c.Render(http.StatusOK, "capd_form.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/capd/logs"))
}

// ---- POST /capd/:id/edit ----

func (h *AuthHandler) CapdUpdate(c echo.Context) error {
	user, profile, err := h.requireCapdPatient(c)
	if user == nil {
		return err
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/capd/logs")
	}

	var existing models.CapdLogEntry
	if err := h.DB.Where("id = ? AND patient_profile_id = ?", id, profile.ID).First(&existing).Error; err != nil {
		return c.Redirect(http.StatusSeeOther, "/capd/logs")
	}

	renderErr := func(msg string) error {
		data := capdFormData(&existing, msg)
		data["IsEditing"] = true
		return c.Render(http.StatusBadRequest, "capd_form.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/capd/logs"))
	}

	logIn, err := parseCapdLogInput(c)
	if err != nil {
		var verr *formValidationError
		if errors.As(err, &verr) {
			return renderErr(verr.msg)
		}
		return err
	}

	existing.LogDate = logIn.LogDate
	existing.CycleNumber = logIn.CycleNumber
	existing.DextroseConcentration = logIn.DextroseConcentration
	existing.FillStartTime = logIn.FillStartTime
	existing.FillEndTime = logIn.FillEndTime
	existing.FillVolumeML = logIn.FillVolumeML
	existing.DrainStartTime = logIn.DrainStartTime
	existing.DrainEndTime = logIn.DrainEndTime
	existing.DrainVolumeML = logIn.DrainVolumeML
	existing.DialysateAppearance = logIn.DialysateAppearance
	existing.WeightKG = logIn.WeightKG
	existing.BPSystolic = logIn.BPSystolic
	existing.BPDiastolic = logIn.BPDiastolic
	existing.UrineOutputML = logIn.UrineOutputML
	existing.ComputeUF()

	if err := h.DB.Save(&existing).Error; err != nil {
		if isDuplicateEntryError(err) {
			return renderErr("มีบันทึกของวันที่และรอบนี้อยู่แล้ว")
		}
		return err
	}

	return c.Redirect(http.StatusSeeOther, "/capd/logs")
}

// ---- POST /capd/:id/delete ----

func (h *AuthHandler) CapdDelete(c echo.Context) error {
	user, profile, err := h.requireCapdPatient(c)
	if user == nil {
		return err
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/capd/logs")
	}

	var entry models.CapdLogEntry
	if err := h.DB.Where("id = ? AND patient_profile_id = ?", id, profile.ID).First(&entry).Error; err != nil {
		return c.Redirect(http.StatusSeeOther, "/capd/logs")
	}

	if err := h.DB.Delete(&entry).Error; err != nil {
		return err
	}

	_ = user
	return c.Redirect(http.StatusSeeOther, "/capd/logs")
}
