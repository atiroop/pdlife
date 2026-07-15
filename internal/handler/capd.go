package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/kpi"
	"github.com/atiroop/pdlife/internal/models"
)

// dialysateAppearanceLabels maps the DB enum to the Thai label shown in
// the UI. The dropdown has no default — the user must pick one every time
// (see docs/schema_spec.md rationale: don't let a default of "clear" hide
// a missed abnormal reading).
var dialysateAppearanceLabels = map[models.DialysateAppearance]string{
	models.DialysateClear:  "ใส",
	models.DialysateCloudy: "ขุ่น",
	models.DialysateBloody: "มีเลือดปน",
}

// DialysateAppearanceLabel returns the Thai label for a dialysate
// appearance value; exported so main.go can register it as a template func.
func DialysateAppearanceLabel(v models.DialysateAppearance) string {
	if label, ok := dialysateAppearanceLabels[v]; ok {
		return label
	}
	return string(v)
}

// requireCapdPatient gates every /capd route: valid session, verified
// email, completed onboarding, and treatment_type == CAPD.
func (h *AuthHandler) requireCapdPatient(c echo.Context) (*models.User, *models.PatientProfile, error) {
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
	if profile.TreatmentType == nil || *profile.TreatmentType != models.TreatmentCAPD {
		return nil, nil, c.Render(http.StatusForbidden, "placeholder.html", map[string]string{
			"Title":   "ใช้ได้เฉพาะผู้ป่วย CAPD",
			"Message": "สมุดบันทึก CAPD ใช้ได้เฉพาะผู้ป่วยที่เลือกวิธีการรักษาแบบ CAPD เท่านั้น",
		})
	}
	return user, &profile, nil
}

// capdDailyAgg is one day's rolled-up totals across all cycles logged that
// day: net UF (sum) and the "representative" weight/BP/urine, taken from
// the day's last cycle (highest cycle_number).
type capdDailyAgg struct {
	Date        time.Time
	UFTotal     int
	LastWeight  float64
	LastBPSys   int
	LastBPDia   int
	LastUrineML *int
	CycleCount  int
}

// aggregateCapdDaily groups cycle entries by log_date. entries must be
// sorted ascending by (log_date, cycle_number) — the caller's query order
// is what makes "last cycle of the day" resolve correctly here, since
// each field is simply overwritten as later cycles are visited.
func aggregateCapdDaily(entries []models.CapdLogEntry) []capdDailyAgg {
	dayIndex := map[string]int{}
	var days []capdDailyAgg
	for _, e := range entries {
		key := e.LogDate.Format("2006-01-02")
		idx, ok := dayIndex[key]
		if !ok {
			days = append(days, capdDailyAgg{Date: e.LogDate})
			idx = len(days) - 1
			dayIndex[key] = idx
		}
		days[idx].UFTotal += e.UFVolumeML
		days[idx].LastWeight = e.WeightKG
		days[idx].LastBPSys = e.BPSystolic
		days[idx].LastBPDia = e.BPDiastolic
		days[idx].CycleCount++
		if e.UrineOutputML != nil {
			days[idx].LastUrineML = e.UrineOutputML
		}
	}
	return days
}

// capdKPICards computes the same KPI cards (+ peritonitis alert) shown at
// the top of /capd — the single source of truth for these
// thresholds/labels, also used by the /dashboard summary card so the two
// can never drift apart.
func (h *AuthHandler) capdKPICards(profileID uint64) (cards []map[string]interface{}, hasLatest bool, latest models.CapdLogEntry, peritonitisAlert bool, peritonitisMeta string) {
	// The single most recently logged cycle, regardless of how old any
	// range toggle a caller might apply elsewhere is — drives the
	// peritonitis banner and the "latest" weight/BP KPI cards.
	hasLatest = h.DB.Where("patient_profile_id = ?", profileID).
		Order("log_date DESC, cycle_number DESC, id DESC").First(&latest).Error == nil

	peritonitisAlert = hasLatest && latest.IsPeritonitisRisk()
	if peritonitisAlert {
		peritonitisMeta = fmt.Sprintf("%s เวลา %s น. (รอบที่ %d)", formatEntryDate(latest.LogDate), latest.DrainEndTime, latest.CycleNumber)
	}

	var latestUrineEntry models.CapdLogEntry
	hasLatestUrine := h.DB.Where("patient_profile_id = ? AND urine_output_ml IS NOT NULL", profileID).
		Order("log_date DESC, cycle_number DESC, id DESC").First(&latestUrineEntry).Error == nil

	today := time.Now().UTC().Truncate(24 * time.Hour)

	var latestDayEntries []models.CapdLogEntry
	latestDayUFTotal := 0
	if hasLatest {
		h.DB.Where("patient_profile_id = ? AND log_date = ?", profileID, latest.LogDate).Find(&latestDayEntries)
		for _, e := range latestDayEntries {
			latestDayUFTotal += e.UFVolumeML
		}
	}

	last7Start := today.AddDate(0, 0, -6)
	var last7Entries []models.CapdLogEntry
	h.DB.Where("patient_profile_id = ? AND log_date >= ?", profileID, last7Start).
		Order("log_date ASC, cycle_number ASC").Find(&last7Entries)
	last7Days := aggregateCapdDaily(last7Entries)

	ufValues := make([]*float64, len(last7Days))
	weightValues := make([]*float64, len(last7Days))
	for i, d := range last7Days {
		ufValues[i] = floatPtr(float64(d.UFTotal))
		weightValues[i] = floatPtr(d.LastWeight)
	}
	avg7dayUF := average(ufValues)
	avg7dayWeight := average(weightValues)

	// Weight status card: latest weight vs. average of the 7 calendar days
	// BEFORE the latest entry's day (same criteria as APD).
	var weightDeltaStatus kpi.Status = kpi.StatusGood
	var weightDeltaKg *float64
	if hasLatest {
		prevWeekStart := latest.LogDate.AddDate(0, 0, -7)
		prevWeekEnd := latest.LogDate.AddDate(0, 0, -1)
		var prevWeekEntries []models.CapdLogEntry
		h.DB.Where("patient_profile_id = ? AND log_date BETWEEN ? AND ?", profileID, prevWeekStart, prevWeekEnd).
			Order("log_date ASC, cycle_number ASC").Find(&prevWeekEntries)
		prevWeekDays := aggregateCapdDaily(prevWeekEntries)
		prevWeightValues := make([]*float64, len(prevWeekDays))
		for i, d := range prevWeekDays {
			prevWeightValues[i] = floatPtr(d.LastWeight)
		}
		if prevAvg := average(prevWeightValues); prevAvg != nil {
			delta := latest.WeightKG - *prevAvg
			weightDeltaKg = &delta
			weightDeltaStatus = kpi.WeightChange(delta)
		}
	}

	cards = []map[string]interface{}{}
	if hasLatest {
		ufStatus := kpi.CapdDailyUF(float64(latestDayUFTotal))
		cards = append(cards, map[string]interface{}{
			"Title":       "UF สุทธิต่อวัน",
			"Value":       fmt.Sprintf("%d", latestDayUFTotal),
			"Unit":        "ml",
			"Meta":        fmt.Sprintf("%s (%d รอบ)", formatEntryDate(latest.LogDate), len(latestDayEntries)),
			"Status":      ufStatus,
			"StatusLabel": ufStatus.Label(),
		})
		weightMeta := formatEntryDate(latest.LogDate)
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
			"Meta":        formatEntryDate(latest.LogDate),
			"Status":      bpStatus,
			"StatusLabel": bpStatus.Label(),
		})
	}
	if hasLatestUrine {
		cards = append(cards, map[string]interface{}{
			"Title":       "ปัสสาวะล่าสุด",
			"Value":       fmt.Sprintf("%d", *latestUrineEntry.UrineOutputML),
			"Unit":        "ml",
			"Meta":        formatEntryDate(latestUrineEntry.LogDate),
			"Status":      kpi.StatusGood,
			"StatusLabel": kpi.StatusGood.Label(),
		})
	}
	if avg7dayUF != nil {
		st := kpi.CapdDailyUF(*avg7dayUF)
		cards = append(cards, map[string]interface{}{
			"Title":       "ค่าเฉลี่ย UF สุทธิ 7 วัน",
			"Value":       fmt.Sprintf("%.0f", *avg7dayUF),
			"Unit":        "ml",
			"Meta":        fmt.Sprintf("%d วันที่มีบันทึก", len(last7Days)),
			"Status":      st,
			"StatusLabel": st.Label(),
		})
	}
	if avg7dayWeight != nil {
		cards = append(cards, map[string]interface{}{
			"Title":       "ค่าเฉลี่ยน้ำหนัก 7 วัน",
			"Value":       fmt.Sprintf("%.1f", *avg7dayWeight),
			"Unit":        "kg",
			"Meta":        fmt.Sprintf("%d วันที่มีบันทึก", len(last7Days)),
			"Status":      kpi.StatusGood,
			"StatusLabel": kpi.StatusGood.Label(),
		})
	}
	return cards, hasLatest, latest, peritonitisAlert, peritonitisMeta
}

// ---- GET /capd ----

func (h *AuthHandler) CapdDashboard(c echo.Context) error {
	user, profile, err := h.requireCapdPatient(c)
	if user == nil {
		return err
	}

	days := 7
	if c.QueryParam("days") == "30" {
		days = 30
	}

	cards, hasLatest, latest, peritonitisAlert, peritonitisMeta := h.capdKPICards(profile.ID)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	rangeStart := today.AddDate(0, 0, -(days - 1))
	var rangeEntries []models.CapdLogEntry
	h.DB.Where("patient_profile_id = ? AND log_date >= ?", profile.ID, rangeStart).
		Order("log_date ASC, cycle_number ASC").Find(&rangeEntries)
	chartDays := aggregateCapdDaily(rangeEntries)

	capdDay := func(d capdDailyAgg) time.Time { return d.Date }
	ufChart := buildDailyTrendSVG(chartDays, "UF สุทธิ/วัน", "ml", capdDay, func(d capdDailyAgg) (float64, bool) {
		return float64(d.UFTotal), true
	}, nil)
	weightChart := buildDailyTrendSVG(chartDays, "น้ำหนัก", "kg", capdDay, func(d capdDailyAgg) (float64, bool) {
		return d.LastWeight, true
	}, nil)

	data := map[string]interface{}{
		"Cards":            cards,
		"HasLatest":        hasLatest,
		"Latest":           latest,
		"LatestAppearance": DialysateAppearanceLabel(latest.DialysateAppearance),
		"PeritonitisAlert": peritonitisAlert,
		"PeritonitisMeta":  peritonitisMeta,
		"Days":             days,
		"UFChart":          ufChart,
		"WeightChart":      weightChart,
		"Disclaimer":       kpi.Disclaimer,
	}
	return c.Render(http.StatusOK, "capd_dashboard.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/capd"))
}

// ---- GET /capd/logs ----

func (h *AuthHandler) CapdLogsList(c echo.Context) error {
	user, profile, err := h.requireCapdPatient(c)
	if user == nil {
		return err
	}

	var logs []models.CapdLogEntry
	h.DB.Where("patient_profile_id = ?", profile.ID).
		Order("log_date DESC, cycle_number DESC, id DESC").Find(&logs)

	rows := make([]map[string]interface{}, len(logs))
	for i, l := range logs {
		rows[i] = map[string]interface{}{
			"Entry":           l,
			"AppearanceLabel": DialysateAppearanceLabel(l.DialysateAppearance),
			"IsRisk":          l.IsPeritonitisRisk(),
		}
	}

	data := map[string]interface{}{"Rows": rows}
	return c.Render(http.StatusOK, "capd_logs.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/capd/logs"))
}
