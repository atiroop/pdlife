package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/labrange"
	"github.com/atiroop/pdlife/internal/models"
)

// labResultInput is the parsed form for one lab-results visit. Every
// field but LogDate is optional — a real lab panel doesn't test
// everything on the same visit (see models.LabResult's doc comment), so
// the form must never require a field just because it exists.
type labResultInput struct {
	LogDate time.Time

	Hct, Hb                               *float64
	WBC, PlateletCount                    *int
	BUN, Cr, Na, K, CO2, Ca, PO4, Albumin *float64
	KtVValue, URR, NPCR                   *float64

	FBS, HbA1C, UricAcid, PTH, Ferritin *float64
	SerumIron, TIBC, TSatPercent        *float64
	Chol, HDL, LDL                      *float64
	HBsAg, HBsAb, AntiHCV, AntiHIV      *models.LabResultFlag
	CXRFinding, EKGFinding              *string

	Notes *string
}

func optionalLabFlag(c echo.Context, key string) *models.LabResultFlag {
	s := strings.TrimSpace(c.FormValue(key))
	switch s {
	case string(models.LabResultNegative):
		v := models.LabResultNegative
		return &v
	case string(models.LabResultPositive):
		v := models.LabResultPositive
		return &v
	default:
		return nil
	}
}

func parseLabResultInput(c echo.Context) (*labResultInput, error) {
	dateStr, err := requiredString(c, "logDate", "วันที่ตรวจ")
	if err != nil {
		return nil, err
	}
	logDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil, &formValidationError{"รูปแบบวันที่ไม่ถูกต้อง"}
	}

	var l labResultInput
	l.LogDate = logDate

	l.Hct = optionalFloat(c, "hct")
	l.Hb = optionalFloat(c, "hb")
	l.WBC = optionalInt(c, "wbc")
	l.PlateletCount = optionalInt(c, "plateletCount")
	l.BUN = optionalFloat(c, "bun")
	l.Cr = optionalFloat(c, "cr")
	l.Na = optionalFloat(c, "na")
	l.K = optionalFloat(c, "k")
	l.CO2 = optionalFloat(c, "co2")
	l.Ca = optionalFloat(c, "ca")
	l.PO4 = optionalFloat(c, "po4")
	l.Albumin = optionalFloat(c, "albumin")
	l.KtVValue = optionalFloat(c, "ktVValue")
	l.URR = optionalFloat(c, "urr")
	l.NPCR = optionalFloat(c, "npcr")

	l.FBS = optionalFloat(c, "fbs")
	l.HbA1C = optionalFloat(c, "hba1c")
	l.UricAcid = optionalFloat(c, "uricAcid")
	l.PTH = optionalFloat(c, "pth")
	l.Ferritin = optionalFloat(c, "ferritin")
	l.SerumIron = optionalFloat(c, "serumIron")
	l.TIBC = optionalFloat(c, "tibc")
	l.TSatPercent = optionalFloat(c, "tSatPercent")
	l.Chol = optionalFloat(c, "chol")
	l.HDL = optionalFloat(c, "hdl")
	l.LDL = optionalFloat(c, "ldl")
	l.HBsAg = optionalLabFlag(c, "hbsag")
	l.HBsAb = optionalLabFlag(c, "hbsab")
	l.AntiHCV = optionalLabFlag(c, "antiHcv")
	l.AntiHIV = optionalLabFlag(c, "antiHiv")
	l.CXRFinding = optionalString(c, "cxrFinding")
	l.EKGFinding = optionalString(c, "ekgFinding")

	l.Notes = optionalString(c, "notes")
	return &l, nil
}

func applyLabResultInput(entry *models.LabResult, in *labResultInput) {
	entry.LogDate = in.LogDate
	entry.Hct, entry.Hb, entry.WBC, entry.PlateletCount = in.Hct, in.Hb, in.WBC, in.PlateletCount
	entry.BUN, entry.Cr, entry.Na, entry.K, entry.CO2, entry.Ca, entry.PO4, entry.Albumin = in.BUN, in.Cr, in.Na, in.K, in.CO2, in.Ca, in.PO4, in.Albumin
	entry.KtVValue, entry.URR, entry.NPCR = in.KtVValue, in.URR, in.NPCR
	entry.FBS, entry.HbA1C, entry.UricAcid, entry.PTH, entry.Ferritin = in.FBS, in.HbA1C, in.UricAcid, in.PTH, in.Ferritin
	entry.SerumIron, entry.TIBC, entry.TSatPercent = in.SerumIron, in.TIBC, in.TSatPercent
	entry.Chol, entry.HDL, entry.LDL = in.Chol, in.HDL, in.LDL
	entry.HBsAg, entry.HBsAb, entry.AntiHCV, entry.AntiHIV = in.HBsAg, in.HBsAb, in.AntiHCV, in.AntiHIV
	entry.CXRFinding, entry.EKGFinding = in.CXRFinding, in.EKGFinding
	entry.Notes = in.Notes
}

func labResultFormData(entry *models.LabResult, isHD bool, formErr string) map[string]interface{} {
	return map[string]interface{}{
		"Log":              entry,
		"Error":            formErr,
		"Today":            time.Now().Format("2006-01-02"),
		"IsHD":             isHD,
		"KtVReferenceText": labrange.KtVReferenceText,
		"Disclaimer":       labrange.Disclaimer,
	}
}

// ---- GET /lab-results/new ----

func (h *AuthHandler) LabResultsNewForm(c echo.Context) error {
	user, profile, err := h.requireLabResultsPatient(c)
	if user == nil {
		return err
	}
	data := labResultFormData(&models.LabResult{}, isHDProfile(profile), "")
	data["IsEditing"] = false
	return c.Render(http.StatusOK, "lab_results_form.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/lab-results/new"))
}

// ---- POST /lab-results/new ----

func (h *AuthHandler) LabResultsCreate(c echo.Context) error {
	user, profile, err := h.requireLabResultsPatient(c)
	if user == nil {
		return err
	}

	renderErr := func(msg string) error {
		data := labResultFormData(nil, isHDProfile(profile), msg)
		data["IsEditing"] = false
		return c.Render(http.StatusBadRequest, "lab_results_form.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/lab-results/new"))
	}

	in, err := parseLabResultInput(c)
	if err != nil {
		var verr *formValidationError
		if errors.As(err, &verr) {
			return renderErr(verr.msg)
		}
		return err
	}

	entry := models.LabResult{PatientProfileID: profile.ID}
	applyLabResultInput(&entry, in)

	if err := h.DB.Create(&entry).Error; err != nil {
		if isDuplicateEntryError(err) {
			return renderErr("มีบันทึกผลตรวจของวันที่นี้อยู่แล้ว")
		}
		return err
	}

	return c.Redirect(http.StatusSeeOther, "/lab-results")
}

// ---- GET /lab-results/:id/edit ----

func (h *AuthHandler) LabResultsEditForm(c echo.Context) error {
	user, profile, err := h.requireLabResultsPatient(c)
	if user == nil {
		return err
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/lab-results")
	}

	var entry models.LabResult
	if err := h.DB.Where("id = ? AND patient_profile_id = ?", id, profile.ID).First(&entry).Error; err != nil {
		return c.Redirect(http.StatusSeeOther, "/lab-results")
	}

	data := labResultFormData(&entry, isHDProfile(profile), "")
	data["IsEditing"] = true
	return c.Render(http.StatusOK, "lab_results_form.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/lab-results"))
}

// ---- POST /lab-results/:id/edit ----

func (h *AuthHandler) LabResultsUpdate(c echo.Context) error {
	user, profile, err := h.requireLabResultsPatient(c)
	if user == nil {
		return err
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/lab-results")
	}

	var existing models.LabResult
	if err := h.DB.Where("id = ? AND patient_profile_id = ?", id, profile.ID).First(&existing).Error; err != nil {
		return c.Redirect(http.StatusSeeOther, "/lab-results")
	}

	renderErr := func(msg string) error {
		data := labResultFormData(&existing, isHDProfile(profile), msg)
		data["IsEditing"] = true
		return c.Render(http.StatusBadRequest, "lab_results_form.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/lab-results"))
	}

	in, err := parseLabResultInput(c)
	if err != nil {
		var verr *formValidationError
		if errors.As(err, &verr) {
			return renderErr(verr.msg)
		}
		return err
	}

	applyLabResultInput(&existing, in)

	if err := h.DB.Save(&existing).Error; err != nil {
		if isDuplicateEntryError(err) {
			return renderErr("มีบันทึกผลตรวจของวันที่นี้อยู่แล้ว")
		}
		return err
	}

	return c.Redirect(http.StatusSeeOther, "/lab-results")
}

// ---- POST /lab-results/:id/delete ----

func (h *AuthHandler) LabResultsDelete(c echo.Context) error {
	user, profile, err := h.requireLabResultsPatient(c)
	if user == nil {
		return err
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/lab-results")
	}

	var entry models.LabResult
	if err := h.DB.Where("id = ? AND patient_profile_id = ?", id, profile.ID).First(&entry).Error; err != nil {
		return c.Redirect(http.StatusSeeOther, "/lab-results")
	}

	if err := h.DB.Delete(&entry).Error; err != nil {
		return err
	}

	_ = user
	return c.Redirect(http.StatusSeeOther, "/lab-results")
}
