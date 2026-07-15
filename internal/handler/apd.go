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
	if profile.HealthDataConsentAt == nil {
		return nil, nil, c.Redirect(http.StatusSeeOther, "/consent")
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

// apdDailyAgg is one day's rolled-up totals across all rounds logged that
// day: Total UF (sum) and the "representative" weight/BP, taken from the
// day's last round (highest cycle_number) — same model as capdDailyAgg.
type apdDailyAgg struct {
	Date       time.Time
	UFTotal    int
	LastWeight float64
	LastBPSys  int
	LastBPDia  int
	CycleCount int
}

// aggregateApdDaily groups round entries by entry_date. entries must be
// sorted ascending by (entry_date, cycle_number) — the caller's query
// order is what makes "last round of the day" resolve correctly here,
// since each field is simply overwritten as later rounds are visited.
func aggregateApdDaily(entries []models.ApdLogEntry) []apdDailyAgg {
	dayIndex := map[string]int{}
	var days []apdDailyAgg
	for _, e := range entries {
		key := e.EntryDate.Format("2006-01-02")
		idx, ok := dayIndex[key]
		if !ok {
			days = append(days, apdDailyAgg{Date: e.EntryDate})
			idx = len(days) - 1
			dayIndex[key] = idx
		}
		days[idx].UFTotal += e.TotalUFML
		days[idx].LastWeight = e.WeightKG
		days[idx].LastBPSys = e.BPSystolic
		days[idx].LastBPDia = e.BPDiastolic
		days[idx].CycleCount++
	}
	return days
}

// apdKPICards computes the same KPI cards shown at the top of /apd — the
// single source of truth for these thresholds/labels, also used by the
// /dashboard summary card so the two can never drift apart. Since patients
// log several rounds per day, UF cards work on daily totals (sum of the
// day's rounds) while weight/BP cards use the most recent round.
func (h *AuthHandler) apdKPICards(profileID uint64) (cards []map[string]interface{}, hasLatest bool, latest models.ApdLogEntry) {
	hasLatest = h.DB.Where("patient_profile_id = ?", profileID).
		Order("entry_date DESC, cycle_number DESC, id DESC").First(&latest).Error == nil

	today := time.Now().UTC().Truncate(24 * time.Hour)
	last7Start := today.AddDate(0, 0, -6)

	var latestDayEntries []models.ApdLogEntry
	latestDayUFTotal := 0
	if hasLatest {
		h.DB.Where("patient_profile_id = ? AND entry_date = ?", profileID, latest.EntryDate).Find(&latestDayEntries)
		for _, e := range latestDayEntries {
			latestDayUFTotal += e.TotalUFML
		}
	}

	var last7Logs []models.ApdLogEntry
	h.DB.Where("patient_profile_id = ? AND entry_date >= ?", profileID, last7Start).
		Order("entry_date ASC, cycle_number ASC").Find(&last7Logs)
	last7Days := aggregateApdDaily(last7Logs)

	ufValues := make([]*float64, len(last7Days))
	for i, d := range last7Days {
		ufValues[i] = floatPtr(float64(d.UFTotal))
	}
	avg7dayUF := average(ufValues)

	// "เทียบเฉลี่ย 7 วันก่อนหน้า" — average of the daily last-round weights
	// over the 7 calendar days BEFORE the latest entry's day.
	var weightDeltaStatus kpi.Status = kpi.StatusGood
	var weightDeltaKg *float64
	if hasLatest {
		prevWeekStart := latest.EntryDate.AddDate(0, 0, -7)
		prevWeekEnd := latest.EntryDate.AddDate(0, 0, -1)
		var prevWeekLogs []models.ApdLogEntry
		h.DB.Where("patient_profile_id = ? AND entry_date BETWEEN ? AND ?", profileID, prevWeekStart, prevWeekEnd).
			Order("entry_date ASC, cycle_number ASC").Find(&prevWeekLogs)
		prevWeekDays := aggregateApdDaily(prevWeekLogs)
		weightValues := make([]*float64, len(prevWeekDays))
		for i, d := range prevWeekDays {
			weightValues[i] = floatPtr(d.LastWeight)
		}
		if prevAvg := average(weightValues); prevAvg != nil {
			delta := latest.WeightKG - *prevAvg
			weightDeltaKg = &delta
			weightDeltaStatus = kpi.WeightChange(delta)
		}
	}

	cards = []map[string]interface{}{}
	if hasLatest {
		ufStatus := kpi.TotalUF(float64(latestDayUFTotal))
		cards = append(cards, map[string]interface{}{
			"Title":       "Total UF ต่อวัน",
			"Value":       fmt.Sprintf("%d", latestDayUFTotal),
			"Unit":        "ml",
			"Meta":        fmt.Sprintf("%s (%d รอบ)", formatEntryDate(latest.EntryDate), len(latestDayEntries)),
			"Status":      ufStatus,
			"StatusLabel": ufStatus.Label(),
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
			"Meta":        fmt.Sprintf("%d วันที่มีบันทึก", len(last7Days)),
			"Status":      st,
			"StatusLabel": st.Label(),
		})
	}
	return cards, hasLatest, latest
}

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

	cards, hasLatest, latest := h.apdKPICards(profile.ID)

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
	days30Start := today.AddDate(0, 0, -(days - 1))
	var chartLogs []models.ApdLogEntry
	h.DB.Where("patient_profile_id = ? AND entry_date >= ?", profile.ID, days30Start).
		Order("entry_date ASC, cycle_number ASC").Find(&chartLogs)
	chartDays := aggregateApdDaily(chartLogs)

	apdDay := func(d apdDailyAgg) time.Time { return d.Date }
	ufChart := buildDailyTrendSVG(chartDays, "Total UF/วัน", "ml", apdDay, func(d apdDailyAgg) (float64, bool) {
		return float64(d.UFTotal), true
	}, nil)
	weightChart := buildDailyTrendSVG(chartDays, "น้ำหนัก", "kg", apdDay, func(d apdDailyAgg) (float64, bool) {
		return d.LastWeight, true
	}, nil)
	bpChart := buildDailyTrendSVG(chartDays, "ความดันโลหิต", "mmHg", apdDay, func(d apdDailyAgg) (float64, bool) {
		return float64(d.LastBPSys), true
	}, func(d apdDailyAgg) (float64, bool) {
		return float64(d.LastBPDia), true
	})

	data := map[string]interface{}{
		"Cards":              cards,
		"HasLatest":          hasLatest,
		"Latest":             latest,
		"LatestPrescription": latestPrescription,
		"Days":               days,
		"UFChart":            ufChart,
		"WeightChart":        weightChart,
		"BPChart":            bpChart,
		"Disclaimer":         kpi.Disclaimer,
	}
	return c.Render(http.StatusOK, "apd_dashboard.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/apd"))
}

// ---- GET /apd/logs ----

func (h *AuthHandler) ApdLogsList(c echo.Context) error {
	user, profile, err := h.requireApdPatient(c)
	if user == nil {
		return err
	}

	var logs []models.ApdLogEntry
	h.DB.Where("patient_profile_id = ?", profile.ID).
		Order("entry_date DESC, cycle_number DESC, id DESC").Find(&logs)

	data := map[string]interface{}{"Rows": logs}
	return c.Render(http.StatusOK, "apd_logs.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/apd/logs"))
}

func formatEntryDate(t time.Time) string {
	thaiMonths := []string{"", "ม.ค.", "ก.พ.", "มี.ค.", "เม.ย.", "พ.ค.", "มิ.ย.", "ก.ค.", "ส.ค.", "ก.ย.", "ต.ค.", "พ.ย.", "ธ.ค."}
	return fmt.Sprintf("%d %s %d", t.Day(), thaiMonths[int(t.Month())], t.Year()+543)
}
