package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"github.com/atiroop/pdlife/internal/models"
)

var defaultPrescriptionValues = models.ApdPrescription{
	Name:               "โปรไฟล์น้ำยาเริ่มต้น",
	SolutionBag1:       "1.5% 5000 ml",
	SolutionBag2:       "2.5% 5000 ml",
	TotalVolumeML:      10000,
	TherapyTimeMinutes: 600,
	FillVolumeML:       2000,
	Cycles:             5,
	DwellTimeMinutes:   90,
	LastFillML:         intPtr(0),
	IsDefaultProfile:   true,
}

func intPtr(v int) *int { return &v }

func (h *AuthHandler) ensureDefaultPrescription(patientProfileID uint64) (*models.ApdPrescription, error) {
	var existing models.ApdPrescription
	err := h.DB.Where("patient_profile_id = ? AND is_default_profile = 1", patientProfileID).
		Order("updated_at DESC").First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	created := defaultPrescriptionValues
	created.PatientProfileID = patientProfileID
	if err := h.DB.Create(&created).Error; err != nil {
		return nil, err
	}
	return &created, nil
}

// formValidationError is a user-facing validation failure; the caller
// re-renders the form with the message instead of a 500.
type formValidationError struct{ msg string }

func (e *formValidationError) Error() string { return e.msg }

func requiredString(c echo.Context, key, label string) (string, error) {
	v := strings.TrimSpace(c.FormValue(key))
	if v == "" {
		return "", &formValidationError{fmt.Sprintf("กรุณากรอก%s", label)}
	}
	return v, nil
}

func optionalString(c echo.Context, key string) *string {
	v := strings.TrimSpace(c.FormValue(key))
	if v == "" {
		return nil
	}
	return &v
}

func requiredInt(c echo.Context, key, label string) (int, error) {
	s, err := requiredString(c, key, label)
	if err != nil {
		return 0, err
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, &formValidationError{fmt.Sprintf("%s ต้องเป็นตัวเลขจำนวนเต็ม", label)}
	}
	return v, nil
}

func optionalInt(c echo.Context, key string) *int {
	s := strings.TrimSpace(c.FormValue(key))
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &v
}

func requiredFloat(c echo.Context, key, label string) (float64, error) {
	s, err := requiredString(c, key, label)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, &formValidationError{fmt.Sprintf("%s ต้องเป็นตัวเลข", label)}
	}
	return v, nil
}

type prescriptionInput struct {
	Name               string
	SolutionBag1       string
	SolutionBag2       string
	TotalVolumeML      int
	TherapyTimeMinutes int
	FillVolumeML       int
	Cycles             int
	DwellTimeMinutes   int
	LastFillML         *int
	ManualExchange     *string
}

func parsePrescriptionInput(c echo.Context) (*prescriptionInput, error) {
	var p prescriptionInput
	var err error
	if p.Name, err = requiredString(c, "prescriptionName", "ชื่อโปรไฟล์"); err != nil {
		return nil, err
	}
	if p.SolutionBag1, err = requiredString(c, "solutionBag1", "น้ำยาถุงที่ 1"); err != nil {
		return nil, err
	}
	if p.SolutionBag2, err = requiredString(c, "solutionBag2", "น้ำยาถุงที่ 2"); err != nil {
		return nil, err
	}
	if p.TotalVolumeML, err = requiredInt(c, "totalVolumeMl", "ปริมาตรรวม"); err != nil {
		return nil, err
	}
	if p.TherapyTimeMinutes, err = requiredInt(c, "therapyTimeMinutes", "เวลารักษารวม"); err != nil {
		return nil, err
	}
	if p.FillVolumeML, err = requiredInt(c, "fillVolumeMl", "ปริมาตรเติมแต่ละรอบ"); err != nil {
		return nil, err
	}
	if p.Cycles, err = requiredInt(c, "cycles", "จำนวนรอบ"); err != nil {
		return nil, err
	}
	if p.DwellTimeMinutes, err = requiredInt(c, "dwellTimeMinutes", "เวลาค้างน้ำยา"); err != nil {
		return nil, err
	}
	p.LastFillML = optionalInt(c, "lastFillMl")
	p.ManualExchange = optionalString(c, "manualExchange")
	return &p, nil
}

// savePrescription mirrors the legacy actions.ts savePrescription(): if
// existingID is set the record in place is updated (used when the caller
// wants to keep reusing one prescription row); otherwise a new row is
// created. When the "default profile" checkbox is on, every other
// prescription for this patient is unmarked as default first.
func (h *AuthHandler) savePrescription(c echo.Context, patientProfileID uint64, existingID *uint64) (*models.ApdPrescription, error) {
	input, err := parsePrescriptionInput(c)
	if err != nil {
		return nil, err
	}
	shouldBeDefault := c.FormValue("isDefaultProfile") == "on"

	if shouldBeDefault {
		if err := h.DB.Model(&models.ApdPrescription{}).
			Where("patient_profile_id = ? AND is_default_profile = 1", patientProfileID).
			Update("is_default_profile", false).Error; err != nil {
			return nil, err
		}
	}

	if existingID != nil {
		var row models.ApdPrescription
		if err := h.DB.First(&row, *existingID).Error; err != nil {
			return nil, err
		}
		row.Name = input.Name
		row.SolutionBag1 = input.SolutionBag1
		row.SolutionBag2 = input.SolutionBag2
		row.TotalVolumeML = input.TotalVolumeML
		row.TherapyTimeMinutes = input.TherapyTimeMinutes
		row.FillVolumeML = input.FillVolumeML
		row.Cycles = input.Cycles
		row.DwellTimeMinutes = input.DwellTimeMinutes
		row.LastFillML = input.LastFillML
		row.ManualExchange = input.ManualExchange
		row.IsDefaultProfile = shouldBeDefault
		if err := h.DB.Save(&row).Error; err != nil {
			return nil, err
		}
		return &row, nil
	}

	row := models.ApdPrescription{
		PatientProfileID:   patientProfileID,
		Name:               input.Name,
		SolutionBag1:       input.SolutionBag1,
		SolutionBag2:       input.SolutionBag2,
		TotalVolumeML:      input.TotalVolumeML,
		TherapyTimeMinutes: input.TherapyTimeMinutes,
		FillVolumeML:       input.FillVolumeML,
		Cycles:             input.Cycles,
		DwellTimeMinutes:   input.DwellTimeMinutes,
		LastFillML:         input.LastFillML,
		ManualExchange:     input.ManualExchange,
		IsDefaultProfile:   shouldBeDefault,
	}
	if err := h.DB.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

type logInput struct {
	EntryDate          time.Time
	TreatmentStartTime string
	WeightKG           float64
	BPSystolic         int
	BPDiastolic        int
	Pulse              int
	BloodGlucoseMgDL   *int
	IDrainVolumeML     int
	TotalUFML          int
	UrineAvgDayML      int
	DrainageAppearance *string
	Remark             *string
}

func parseLogInput(c echo.Context) (*logInput, error) {
	dateStr, err := requiredString(c, "date", "วันที่")
	if err != nil {
		return nil, err
	}
	entryDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil, &formValidationError{"รูปแบบวันที่ไม่ถูกต้อง"}
	}

	var l logInput
	l.EntryDate = entryDate
	if l.TreatmentStartTime, err = requiredString(c, "treatmentStartTime", "เวลาเริ่มทำ APD"); err != nil {
		return nil, err
	}
	if l.WeightKG, err = requiredFloat(c, "weightKg", "น้ำหนัก"); err != nil {
		return nil, err
	}
	if l.BPSystolic, err = requiredInt(c, "systolicBp", "ความดันตัวบน"); err != nil {
		return nil, err
	}
	if l.BPDiastolic, err = requiredInt(c, "diastolicBp", "ความดันตัวล่าง"); err != nil {
		return nil, err
	}
	if l.Pulse, err = requiredInt(c, "pulse", "ชีพจร"); err != nil {
		return nil, err
	}
	l.BloodGlucoseMgDL = optionalInt(c, "bloodGlucoseMgDl")
	if l.IDrainVolumeML, err = requiredInt(c, "iDrainVolumeMl", "ปริมาณ I-Drain"); err != nil {
		return nil, err
	}
	if l.TotalUFML, err = requiredInt(c, "totalUfMl", "Total UF"); err != nil {
		return nil, err
	}
	if l.UrineAvgDayML, err = requiredInt(c, "urineAvgDayMl", "ปัสสาวะเฉลี่ยต่อวัน"); err != nil {
		return nil, err
	}
	l.DrainageAppearance = optionalString(c, "drainageAppearance")
	l.Remark = optionalString(c, "remark")
	return &l, nil
}

func (h *AuthHandler) apdFormData(profile *models.PatientProfile, log *models.ApdLogEntry, formErr string) (map[string]interface{}, error) {
	var prescription models.ApdPrescription
	if log != nil && log.PrescriptionID != nil {
		if err := h.DB.First(&prescription, *log.PrescriptionID).Error; err != nil {
			return nil, err
		}
	} else {
		p, err := h.ensureDefaultPrescription(profile.ID)
		if err != nil {
			return nil, err
		}
		prescription = *p
	}

	return map[string]interface{}{
		"Log":                       log,
		"Prescription":              prescription,
		"Error":                     formErr,
		"Today":                     time.Now().Format("2006-01-02"),
		"DrainageAppearanceOptions": drainageAppearanceOptions,
	}, nil
}

// ---- GET /apd/new ----

func (h *AuthHandler) ApdNewForm(c echo.Context) error {
	user, profile, err := h.requireApdPatient(c)
	if user == nil {
		return err
	}
	data, err := h.apdFormData(profile, nil, "")
	if err != nil {
		return err
	}
	data["User"] = user
	data["IsEditing"] = false
	return c.Render(http.StatusOK, "apd_form.html", data)
}

// ---- POST /apd/new ----

func (h *AuthHandler) ApdCreate(c echo.Context) error {
	user, profile, err := h.requireApdPatient(c)
	if user == nil {
		return err
	}

	defaultPrescription, err := h.ensureDefaultPrescription(profile.ID)
	if err != nil {
		return err
	}
	useDefault := c.FormValue("isDefaultProfile") == "on"
	var existingID *uint64
	if useDefault {
		existingID = &defaultPrescription.ID
	}

	renderErr := func(msg string) error {
		data, ferr := h.apdFormData(profile, nil, msg)
		if ferr != nil {
			return ferr
		}
		data["User"] = user
		data["IsEditing"] = false
		return c.Render(http.StatusBadRequest, "apd_form.html", data)
	}

	prescription, err := h.savePrescription(c, profile.ID, existingID)
	if err != nil {
		var verr *formValidationError
		if errors.As(err, &verr) {
			return renderErr(verr.msg)
		}
		return err
	}

	logIn, err := parseLogInput(c)
	if err != nil {
		var verr *formValidationError
		if errors.As(err, &verr) {
			return renderErr(verr.msg)
		}
		return err
	}

	entry := models.ApdLogEntry{
		PatientProfileID:   profile.ID,
		EntryDate:          logIn.EntryDate,
		TreatmentStartTime: logIn.TreatmentStartTime,
		WeightKG:           logIn.WeightKG,
		BPSystolic:         logIn.BPSystolic,
		BPDiastolic:        logIn.BPDiastolic,
		Pulse:              logIn.Pulse,
		BloodGlucoseMgDL:   logIn.BloodGlucoseMgDL,
		IDrainVolumeML:     logIn.IDrainVolumeML,
		TotalUFML:          logIn.TotalUFML,
		UrineAvgDayML:      logIn.UrineAvgDayML,
		DrainageAppearance: logIn.DrainageAppearance,
		Remark:             logIn.Remark,
		PrescriptionID:     &prescription.ID,
	}
	if err := h.DB.Create(&entry).Error; err != nil {
		if isDuplicateEntryError(err) {
			return renderErr("มีบันทึกของวันที่นี้อยู่แล้ว")
		}
		return err
	}

	return c.Redirect(http.StatusSeeOther, "/apd")
}

// ---- GET /apd/:id/edit ----

func (h *AuthHandler) ApdEditForm(c echo.Context) error {
	user, profile, err := h.requireApdPatient(c)
	if user == nil {
		return err
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/apd/logs")
	}

	var log models.ApdLogEntry
	if err := h.DB.Where("id = ? AND patient_profile_id = ?", id, profile.ID).First(&log).Error; err != nil {
		return c.Redirect(http.StatusSeeOther, "/apd/logs")
	}

	data, err := h.apdFormData(profile, &log, "")
	if err != nil {
		return err
	}
	data["User"] = user
	data["IsEditing"] = true
	return c.Render(http.StatusOK, "apd_form.html", data)
}

// ---- POST /apd/:id/edit ----

func (h *AuthHandler) ApdUpdate(c echo.Context) error {
	user, profile, err := h.requireApdPatient(c)
	if user == nil {
		return err
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/apd/logs")
	}

	var existing models.ApdLogEntry
	if err := h.DB.Where("id = ? AND patient_profile_id = ?", id, profile.ID).First(&existing).Error; err != nil {
		return c.Redirect(http.StatusSeeOther, "/apd/logs")
	}

	renderErr := func(msg string) error {
		data, ferr := h.apdFormData(profile, &existing, msg)
		if ferr != nil {
			return ferr
		}
		data["User"] = user
		data["IsEditing"] = true
		return c.Render(http.StatusBadRequest, "apd_form.html", data)
	}

	// Legacy behaviour: editing always updates whichever prescription this
	// log entry currently points to, in place (never spawns a new row).
	prescription, err := h.savePrescription(c, profile.ID, existing.PrescriptionID)
	if err != nil {
		var verr *formValidationError
		if errors.As(err, &verr) {
			return renderErr(verr.msg)
		}
		return err
	}

	logIn, err := parseLogInput(c)
	if err != nil {
		var verr *formValidationError
		if errors.As(err, &verr) {
			return renderErr(verr.msg)
		}
		return err
	}

	existing.EntryDate = logIn.EntryDate
	existing.TreatmentStartTime = logIn.TreatmentStartTime
	existing.WeightKG = logIn.WeightKG
	existing.BPSystolic = logIn.BPSystolic
	existing.BPDiastolic = logIn.BPDiastolic
	existing.Pulse = logIn.Pulse
	existing.BloodGlucoseMgDL = logIn.BloodGlucoseMgDL
	existing.IDrainVolumeML = logIn.IDrainVolumeML
	existing.TotalUFML = logIn.TotalUFML
	existing.UrineAvgDayML = logIn.UrineAvgDayML
	existing.DrainageAppearance = logIn.DrainageAppearance
	existing.Remark = logIn.Remark
	existing.PrescriptionID = &prescription.ID

	if err := h.DB.Save(&existing).Error; err != nil {
		if isDuplicateEntryError(err) {
			return renderErr("มีบันทึกของวันที่นี้อยู่แล้ว")
		}
		return err
	}

	return c.Redirect(http.StatusSeeOther, "/apd/logs")
}

// ---- POST /apd/:id/delete ----

func (h *AuthHandler) ApdDelete(c echo.Context) error {
	user, profile, err := h.requireApdPatient(c)
	if user == nil {
		return err
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/apd/logs")
	}

	var log models.ApdLogEntry
	if err := h.DB.Where("id = ? AND patient_profile_id = ?", id, profile.ID).First(&log).Error; err != nil {
		return c.Redirect(http.StatusSeeOther, "/apd/logs")
	}

	if err := h.DB.Delete(&log).Error; err != nil {
		return err
	}

	if log.PrescriptionID != nil {
		var usageCount int64
		h.DB.Model(&models.ApdLogEntry{}).Where("prescription_id = ?", *log.PrescriptionID).Count(&usageCount)
		if usageCount == 0 {
			var prescription models.ApdPrescription
			if h.DB.First(&prescription, *log.PrescriptionID).Error == nil && !prescription.IsDefaultProfile {
				h.DB.Delete(&prescription)
			}
		}
	}

	_ = user
	return c.Redirect(http.StatusSeeOther, "/apd/logs")
}

func isDuplicateEntryError(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
