package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/kpi"
	"github.com/atiroop/pdlife/internal/models"
)

// drainageAppearanceOptions mirrors the fixed option list from the legacy
// system (see docs/schema_spec.md) — drainage_appearance is free text in
// the DB but the form offers these as presets plus "other".
var drainageAppearanceOptions = []string{
	"ใส", "เหลืองอ่อน", "ขุ่น", "มีเส้นไฟบริน", "ชมพู/มีเลือดปน", "อื่น ๆ - ดูหมายเหตุ",
}

// requireApdPatient gates every /apd route: valid session, verified email,
// completed onboarding, and treatment_type == APD (see auth_flow_spec.md).
// If user is nil the guard has already written a response (redirect or
// render) and the caller should return err as-is.
func (h *AuthHandler) requireApdPatient(c echo.Context) (*models.User, *models.PatientProfile, error) {
	user, err := h.currentSession(c)
	if err != nil {
		return nil, nil, c.Redirect(http.StatusSeeOther, "/login")
	}
	if user.Role == models.RoleUnverified {
		return nil, nil, c.Render(http.StatusForbidden, "placeholder.html", map[string]string{
			"Title":   "ยืนยันอีเมลก่อน",
			"Message": "กรุณายืนยันอีเมลก่อนใช้งานสมุดบันทึก",
		})
	}

	var profile models.PatientProfile
	if err := h.DB.Where("user_id = ?", user.ID).First(&profile).Error; err != nil || profile.ProfileCompletedAt == nil {
		return nil, nil, c.Redirect(http.StatusSeeOther, "/onboarding")
	}
	if profile.TreatmentType == nil || *profile.TreatmentType != models.TreatmentAPD {
		return nil, nil, c.Render(http.StatusForbidden, "placeholder.html", map[string]string{
			"Title":   "ใช้ได้เฉพาะผู้ป่วย APD",
			"Message": "สมุดบันทึก APD ใช้ได้เฉพาะผู้ป่วยที่เลือกวิธีการรักษาแบบ APD เท่านั้น",
		})
	}
	return user, &profile, nil
}

// average mirrors the legacy _lib.ts average(): arithmetic mean of the
// non-nil values, or nil if there are none.
func average(values []*float64) *float64 {
	var sum float64
	var count int
	for _, v := range values {
		if v != nil {
			sum += *v
			count++
		}
	}
	if count == 0 {
		return nil
	}
	avg := sum / float64(count)
	return &avg
}

func floatPtr(v float64) *float64 { return &v }

// ---- GET /apd ----

func (h *AuthHandler) ApdDashboard(c echo.Context) error {
	user, profile, err := h.requireApdPatient(c)
	if user == nil {
		return err
	}

	days := 7
	if c.QueryParam("days") == "30" {
		days = 30
	}

	var latest models.ApdLogEntry
	hasLatest := h.DB.Where("patient_profile_id = ?", profile.ID).
		Order("entry_date DESC, id DESC").First(&latest).Error == nil

	var latestPrescription *models.ApdPrescription
	if hasLatest && latest.PrescriptionID != nil {
		var p models.ApdPrescription
		if h.DB.First(&p, *latest.PrescriptionID).Error == nil {
			latestPrescription = &p
		}
	}
	if latestPrescription == nil {
		var p models.ApdPrescription
		if h.DB.Where("patient_profile_id = ? AND is_default_profile = 1", profile.ID).
			Order("updated_at DESC").First(&p).Error == nil {
			latestPrescription = &p
		}
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	last7Start := today.AddDate(0, 0, -6)

	var last7Logs []models.ApdLogEntry
	h.DB.Where("patient_profile_id = ? AND entry_date >= ?", profile.ID, last7Start).
		Order("entry_date ASC").Find(&last7Logs)

	ufValues := make([]*float64, len(last7Logs))
	for i, l := range last7Logs {
		ufValues[i] = floatPtr(float64(l.TotalUFML))
	}
	avg7dayUF := average(ufValues)

	// "เทียบเฉลี่ย 7 วันก่อนหน้า" — average of the 7 calendar days BEFORE
	// the latest entry (excluding the latest entry itself).
	var weightDeltaStatus kpi.Status = kpi.StatusGood
	var weightDeltaKg *float64
	if hasLatest {
		prevWeekStart := latest.EntryDate.AddDate(0, 0, -7)
		prevWeekEnd := latest.EntryDate.AddDate(0, 0, -1)
		var prevWeekLogs []models.ApdLogEntry
		h.DB.Where("patient_profile_id = ? AND entry_date BETWEEN ? AND ?", profile.ID, prevWeekStart, prevWeekEnd).
			Find(&prevWeekLogs)
		weightValues := make([]*float64, len(prevWeekLogs))
		for i, l := range prevWeekLogs {
			weightValues[i] = floatPtr(l.WeightKG)
		}
		if prevAvg := average(weightValues); prevAvg != nil {
			delta := latest.WeightKG - *prevAvg
			weightDeltaKg = &delta
			weightDeltaStatus = kpi.WeightChange(delta)
		}
	}

	days30Start := today.AddDate(0, 0, -(days - 1))
	var chartLogs []models.ApdLogEntry
	h.DB.Where("patient_profile_id = ? AND entry_date >= ?", profile.ID, days30Start).
		Order("entry_date ASC").Find(&chartLogs)

	cards := []map[string]interface{}{}
	if hasLatest {
		cards = append(cards, map[string]interface{}{
			"Title":       "Total UF ล่าสุด",
			"Value":       fmt.Sprintf("%d", latest.TotalUFML),
			"Unit":        "ml",
			"Meta":        formatEntryDate(latest.EntryDate),
			"Status":      kpi.TotalUF(float64(latest.TotalUFML)),
			"StatusLabel": kpi.TotalUF(float64(latest.TotalUFML)).Label(),
		})
		weightMeta := formatEntryDate(latest.EntryDate)
		if weightDeltaKg != nil {
			weightMeta = fmt.Sprintf("%+.1f kg จากค่าเฉลี่ย 7 วันก่อนหน้า", *weightDeltaKg)
		}
		cards = append(cards, map[string]interface{}{
			"Title":       "น้ำหนักล่าสุด",
			"Value":       fmt.Sprintf("%.1f", latest.WeightKG),
			"Unit":        "kg",
			"Meta":        weightMeta,
			"Status":      weightDeltaStatus,
			"StatusLabel": weightDeltaStatus.Label(),
		})
		bpStatus := kpi.BloodPressure(latest.BPSystolic, latest.BPDiastolic)
		cards = append(cards, map[string]interface{}{
			"Title":       "ความดันล่าสุด",
			"Value":       fmt.Sprintf("%d/%d", latest.BPSystolic, latest.BPDiastolic),
			"Unit":        "mmHg",
			"Meta":        fmt.Sprintf("ชีพจร %d", latest.Pulse),
			"Status":      bpStatus,
			"StatusLabel": bpStatus.Label(),
		})
	}
	if avg7dayUF != nil {
		st := kpi.TotalUF(*avg7dayUF)
		cards = append(cards, map[string]interface{}{
			"Title":       "ค่าเฉลี่ย Total UF 7 วัน",
			"Value":       fmt.Sprintf("%.0f", *avg7dayUF),
			"Unit":        "ml",
			"Meta":        fmt.Sprintf("%d วันที่มีบันทึก", len(last7Logs)),
			"Status":      st,
			"StatusLabel": st.Label(),
		})
	}

	ufChart := buildTrendSVG(chartLogs, "Total UF", "ml", func(l models.ApdLogEntry) (float64, bool) {
		return float64(l.TotalUFML), true
	}, nil)
	weightChart := buildTrendSVG(chartLogs, "น้ำหนัก", "kg", func(l models.ApdLogEntry) (float64, bool) {
		return l.WeightKG, true
	}, nil)
	bpChart := buildTrendSVG(chartLogs, "ความดันโลหิต", "mmHg", func(l models.ApdLogEntry) (float64, bool) {
		return float64(l.BPSystolic), true
	}, func(l models.ApdLogEntry) (float64, bool) {
		return float64(l.BPDiastolic), true
	})

	return c.Render(http.StatusOK, "apd_dashboard.html", map[string]interface{}{
		"User":               user,
		"Cards":              cards,
		"HasLatest":          hasLatest,
		"Latest":             latest,
		"LatestPrescription": latestPrescription,
		"Days":               days,
		"UFChart":            ufChart,
		"WeightChart":        weightChart,
		"BPChart":            bpChart,
	})
}

// ---- GET /apd/logs ----

func (h *AuthHandler) ApdLogsList(c echo.Context) error {
	user, profile, err := h.requireApdPatient(c)
	if user == nil {
		return err
	}

	var logs []models.ApdLogEntry
	h.DB.Where("patient_profile_id = ?", profile.ID).
		Order("entry_date DESC, id DESC").Find(&logs)

	prescriptionNames := map[uint64]string{}
	var prescriptions []models.ApdPrescription
	h.DB.Where("patient_profile_id = ?", profile.ID).Find(&prescriptions)
	for _, p := range prescriptions {
		prescriptionNames[p.ID] = p.Name
	}

	type row struct {
		models.ApdLogEntry
		PrescriptionName string
	}
	rows := make([]row, len(logs))
	for i, l := range logs {
		name := ""
		if l.PrescriptionID != nil {
			name = prescriptionNames[*l.PrescriptionID]
		}
		rows[i] = row{ApdLogEntry: l, PrescriptionName: name}
	}

	return c.Render(http.StatusOK, "apd_logs.html", map[string]interface{}{
		"User": user,
		"Rows": rows,
	})
}

func formatEntryDate(t time.Time) string {
	thaiMonths := []string{"", "ม.ค.", "ก.พ.", "มี.ค.", "เม.ย.", "พ.ค.", "มิ.ย.", "ก.ค.", "ส.ค.", "ก.ย.", "ต.ค.", "พ.ย.", "ธ.ค."}
	return fmt.Sprintf("%d %s %d", t.Day(), thaiMonths[int(t.Month())], t.Year()+543)
}
