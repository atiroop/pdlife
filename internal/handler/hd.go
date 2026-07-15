package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/kpi"
	"github.com/atiroop/pdlife/internal/models"
)

// requireHdPatient gates every /hd route: valid session, verified email,
// completed onboarding, health-data consent, and treatment_type == HD.
func (h *AuthHandler) requireHdPatient(c echo.Context) (*models.User, *models.PatientProfile, error) {
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
	if profile.TreatmentType == nil || *profile.TreatmentType != models.TreatmentHD {
		return nil, nil, c.Render(http.StatusForbidden, "placeholder.html", map[string]string{
			"Title":   "ใช้ได้เฉพาะผู้ป่วย HD",
			"Message": "สมุดบันทึก HD ใช้ได้เฉพาะผู้ป่วยที่เลือกวิธีการรักษาแบบ HD เท่านั้น",
		})
	}
	return user, &profile, nil
}

// hdKPICards computes the same KPI cards shown at the top of /hd — the
// single source of truth for these thresholds/labels, also used by the
// /dashboard summary card so the two can never drift apart.
func (h *AuthHandler) hdKPICards(profileID uint64) (cards []map[string]interface{}, hasLatest bool, latest models.HdLogEntry) {
	hasLatest = h.DB.Where("patient_profile_id = ?", profileID).
		Order("log_date DESC, id DESC").First(&latest).Error == nil

	today := time.Now().UTC().Truncate(24 * time.Hour)
	last7Start := today.AddDate(0, 0, -6)

	var last7Logs []models.HdLogEntry
	h.DB.Where("patient_profile_id = ? AND log_date >= ?", profileID, last7Start).
		Order("log_date ASC").Find(&last7Logs)

	ufValues := make([]*float64, len(last7Logs))
	for i, l := range last7Logs {
		ufValues[i] = floatPtr(float64(l.UFRemovedML))
	}
	avg7dayUF := average(ufValues)

	// น้ำหนักเพิ่มระหว่างรอบ: this session's pre-dialysis weight minus the
	// immediately preceding session's post-dialysis weight.
	var interdialyticGainKg *float64
	gainStatus := kpi.StatusGood
	if hasLatest {
		var prev models.HdLogEntry
		if h.DB.Where("patient_profile_id = ? AND log_date < ?", profileID, latest.LogDate).
			Order("log_date DESC, id DESC").First(&prev).Error == nil {
			gain := latest.PreDialysisWeightKG - prev.PostDialysisWeightKG
			interdialyticGainKg = &gain
			gainStatus = kpi.HDInterdialyticGain(gain)
		}
	}

	cards = []map[string]interface{}{}
	if hasLatest {
		postVsDry := latest.PostDialysisWeightKG - latest.DryWeightKG
		postVsDryStatus := kpi.HDPostVsDry(postVsDry)
		cards = append(cards, map[string]interface{}{
			"Title":       "น้ำหนักหลังฟอกล่าสุด",
			"Value":       fmt.Sprintf("%.1f", latest.PostDialysisWeightKG),
			"Unit":        "kg",
			"Meta":        fmt.Sprintf("%+.1f kg จากน้ำหนักแห้ง (%s)", postVsDry, formatEntryDate(latest.LogDate)),
			"Status":      postVsDryStatus,
			"StatusLabel": postVsDryStatus.Label(),
		})

		preBPStatus := kpi.BloodPressure(latest.PreDialysisBPSystolic, latest.PreDialysisBPDiastolic)
		cards = append(cards, map[string]interface{}{
			"Title":       "ความดันก่อนฟอกล่าสุด",
			"Value":       fmt.Sprintf("%d/%d", latest.PreDialysisBPSystolic, latest.PreDialysisBPDiastolic),
			"Unit":        "mmHg",
			"Meta":        formatEntryDate(latest.LogDate),
			"Status":      preBPStatus,
			"StatusLabel": preBPStatus.Label(),
		})

		postBPStatus := kpi.HDPostDialysisBP(latest.PostDialysisBPSystolic)
		cards = append(cards, map[string]interface{}{
			"Title":       "ความดันหลังฟอกล่าสุด",
			"Value":       fmt.Sprintf("%d/%d", latest.PostDialysisBPSystolic, latest.PostDialysisBPDiastolic),
			"Unit":        "mmHg",
			"Meta":        formatEntryDate(latest.LogDate),
			"Status":      postBPStatus,
			"StatusLabel": postBPStatus.Label(),
		})

		cards = append(cards, map[string]interface{}{
			"Title":       "UF ที่ดึงออกล่าสุด",
			"Value":       fmt.Sprintf("%d", latest.UFRemovedML),
			"Unit":        "ml",
			"Meta":        formatEntryDate(latest.LogDate),
			"Status":      kpi.StatusGood,
			"StatusLabel": kpi.StatusGood.Label(),
		})

		if interdialyticGainKg != nil {
			cards = append(cards, map[string]interface{}{
				"Title":       "น้ำหนักเพิ่มระหว่างรอบ",
				"Value":       fmt.Sprintf("%+.1f", *interdialyticGainKg),
				"Unit":        "kg",
				"Meta":        "เทียบกับน้ำหนักหลังฟอกครั้งก่อน",
				"Status":      gainStatus,
				"StatusLabel": gainStatus.Label(),
			})
		}
	}
	if avg7dayUF != nil {
		cards = append(cards, map[string]interface{}{
			"Title":       "ค่าเฉลี่ย UF 7 วัน",
			"Value":       fmt.Sprintf("%.0f", *avg7dayUF),
			"Unit":        "ml",
			"Meta":        fmt.Sprintf("%d ครั้งที่มีบันทึก", len(last7Logs)),
			"Status":      kpi.StatusGood,
			"StatusLabel": kpi.StatusGood.Label(),
		})
	}
	return cards, hasLatest, latest
}

// ---- GET /hd ----

func (h *AuthHandler) HdDashboard(c echo.Context) error {
	user, profile, err := h.requireHdPatient(c)
	if user == nil {
		return err
	}

	days := 7
	if c.QueryParam("days") == "30" {
		days = 30
	}

	cards, hasLatest, latest := h.hdKPICards(profile.ID)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	rangeStart := today.AddDate(0, 0, -(days - 1))
	var chartLogs []models.HdLogEntry
	h.DB.Where("patient_profile_id = ? AND log_date >= ?", profile.ID, rangeStart).
		Order("log_date ASC").Find(&chartLogs)

	weightChart := buildHdTrendSVG(chartLogs, "น้ำหนัก (ก่อน/หลัง/แห้ง)", "kg",
		func(l models.HdLogEntry) (float64, bool) { return l.PreDialysisWeightKG, true },
		func(l models.HdLogEntry) (float64, bool) { return l.PostDialysisWeightKG, true },
		func(l models.HdLogEntry) (float64, bool) { return l.DryWeightKG, true },
	)
	bpChart := buildHdTrendSVG(chartLogs, "ความดันโลหิต (ก่อน/หลังฟอก)", "mmHg",
		func(l models.HdLogEntry) (float64, bool) { return float64(l.PreDialysisBPSystolic), true },
		func(l models.HdLogEntry) (float64, bool) { return float64(l.PostDialysisBPSystolic), true },
		nil,
	)
	ufChart := buildHdTrendSVG(chartLogs, "UF ที่ดึงออก", "ml",
		func(l models.HdLogEntry) (float64, bool) { return float64(l.UFRemovedML), true },
		nil, nil,
	)

	data := map[string]interface{}{
		"Cards":       cards,
		"HasLatest":   hasLatest,
		"Latest":      latest,
		"Days":        days,
		"WeightChart": weightChart,
		"BPChart":     bpChart,
		"UFChart":     ufChart,
		"Disclaimer":  kpi.Disclaimer,
	}
	return c.Render(http.StatusOK, "hd_dashboard.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/hd"))
}

// ---- GET /hd/logs ----

func (h *AuthHandler) HdLogsList(c echo.Context) error {
	user, profile, err := h.requireHdPatient(c)
	if user == nil {
		return err
	}

	var logs []models.HdLogEntry
	h.DB.Where("patient_profile_id = ?", profile.ID).
		Order("log_date DESC, id DESC").Find(&logs)

	data := map[string]interface{}{"Rows": logs}
	return c.Render(http.StatusOK, "hd_logs.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/hd/logs"))
}
