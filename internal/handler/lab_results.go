package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/labrange"
	"github.com/atiroop/pdlife/internal/models"
)

// requireLabResultsPatient gates every /lab-results route: valid session,
// verified email, completed onboarding, health-data consent. Unlike
// requireApdPatient/requireCapdPatient/requireHdPatient, this deliberately
// does NOT check treatment_type — lab results apply to every treatment
// type (see Phase 6 spec: "ไม่ผูก gate แบบ APD/CAPD/HD เดิม").
func (h *AuthHandler) requireLabResultsPatient(c echo.Context) (*models.User, *models.PatientProfile, error) {
	user, err := h.currentSession(c)
	if err != nil {
		return nil, nil, c.Redirect(http.StatusSeeOther, "/login")
	}
	if user.Role == models.RoleUnverified {
		return nil, nil, c.Render(http.StatusForbidden, "placeholder.html", map[string]string{
			"Title":   "ยืนยันอีเมลก่อน",
			"Message": "กรุณายืนยันอีเมลก่อนใช้งานผลตรวจเลือด",
		})
	}
	var profile models.PatientProfile
	if err := h.DB.Where("user_id = ?", user.ID).First(&profile).Error; err != nil || profile.ProfileCompletedAt == nil {
		return nil, nil, c.Redirect(http.StatusSeeOther, "/onboarding")
	}
	if profile.HealthDataConsentAt == nil {
		return nil, nil, c.Redirect(http.StatusSeeOther, "/consent")
	}
	return user, &profile, nil
}

func isHDProfile(profile *models.PatientProfile) bool {
	return profile.TreatmentType != nil && *profile.TreatmentType == models.TreatmentHD
}

// labNumericField describes one numeric lab value: how to read it off a
// LabResult row, its display label/unit, and its reference range. Driving
// the abnormal-values summary and the logs table from one list keeps the
// ~25 fields from drifting out of sync with each other.
type labNumericField struct {
	Key   string
	Label string
	Unit  string
	Range labrange.Range
	Get   func(models.LabResult) *float64
}

func intToFloatPtr(p *int) *float64 {
	if p == nil {
		return nil
	}
	v := float64(*p)
	return &v
}

// labNumericFields covers every non-HD-only numeric value, both the
// every-3-months and every-6-months/1-year groups (they're treated
// identically for abnormal-detection purposes — only the form UI splits
// them into two accordion sections).
var labNumericFields = []labNumericField{
	{"hct", "Hct", "%", labrange.HctRange, func(r models.LabResult) *float64 { return r.Hct }},
	{"hb", "Hb", "g/dL", labrange.HbRange, func(r models.LabResult) *float64 { return r.Hb }},
	{"wbc", "WBC", "/mm³", labrange.WBCRange, func(r models.LabResult) *float64 { return intToFloatPtr(r.WBC) }},
	{"platelet_count", "Platelet", "/mm³", labrange.PlateletRange, func(r models.LabResult) *float64 { return intToFloatPtr(r.PlateletCount) }},
	{"bun", "BUN", "mg/dL", labrange.BUNRange, func(r models.LabResult) *float64 { return r.BUN }},
	{"cr", "Cr", "mg/dL", labrange.CrRange, func(r models.LabResult) *float64 { return r.Cr }},
	{"na", "Na", "mEq/L", labrange.NaRange, func(r models.LabResult) *float64 { return r.Na }},
	{"k", "K", "mEq/L", labrange.KRange, func(r models.LabResult) *float64 { return r.K }},
	{"co2", "CO2", "mEq/L", labrange.CO2Range, func(r models.LabResult) *float64 { return r.CO2 }},
	{"ca", "Ca", "mg/dL", labrange.CaRange, func(r models.LabResult) *float64 { return r.Ca }},
	{"po4", "PO4", "mg/dL", labrange.PO4Range, func(r models.LabResult) *float64 { return r.PO4 }},
	{"albumin", "Albumin", "g/dL", labrange.AlbuminRange, func(r models.LabResult) *float64 { return r.Albumin }},
	{"fbs", "FBS", "mg/dL", labrange.FBSRange, func(r models.LabResult) *float64 { return r.FBS }},
	{"hba1c", "HbA1C", "%", labrange.HbA1CRange, func(r models.LabResult) *float64 { return r.HbA1C }},
	{"uric_acid", "Uric acid", "mg/dL", labrange.UricAcidRange, func(r models.LabResult) *float64 { return r.UricAcid }},
	{"pth", "PTH", "pg/mL", labrange.PTHRange, func(r models.LabResult) *float64 { return r.PTH }},
	{"ferritin", "Ferritin", "ng/mL", labrange.FerritinRange, func(r models.LabResult) *float64 { return r.Ferritin }},
	{"serum_iron", "Serum iron", "mcg/dL", labrange.SerumIronRange, func(r models.LabResult) *float64 { return r.SerumIron }},
	{"tibc", "TIBC", "mcg/dL", labrange.TIBCRange, func(r models.LabResult) *float64 { return r.TIBC }},
	{"t_sat_percent", "%T sat", "%", labrange.TSatRange, func(r models.LabResult) *float64 { return r.TSatPercent }},
	{"chol", "Chol", "mg/dL", labrange.CholRange, func(r models.LabResult) *float64 { return r.Chol }},
	{"hdl", "HDL", "mg/dL", labrange.HDLRange, func(r models.LabResult) *float64 { return r.HDL }},
	{"ldl", "LDL", "mg/dL", labrange.LDLRange, func(r models.LabResult) *float64 { return r.LDL }},
}

// labHDNumericFields are evaluated for abnormal-detection only when the
// patient's treatment type is HD (URR, nPCR). Kt/V is tracked separately
// (see labrange.KtVReferenceText) since it's never auto-classified.
var labHDNumericFields = []labNumericField{
	{"urr", "URR", "%", labrange.URRRange, func(r models.LabResult) *float64 { return r.URR }},
	{"npcr", "nPCR", "g/kg/day", labrange.NPCRRange, func(r models.LabResult) *float64 { return r.NPCR }},
}

type labEnumField struct {
	Key          string
	Label        string
	NormalValue  models.LabResultFlag
	AbnormalNote string
	Get          func(models.LabResult) *models.LabResultFlag
}

// labEnumFields: HBsAg/Anti HCV/Anti HIV are normal when negative; HBsAb
// is normal when positive (it's an immunity marker, not an infection
// marker) — see Phase 6 spec.
var labEnumFields = []labEnumField{
	{"hbsag", "HBsAg", models.LabResultNegative, "พบเชื้อไวรัสตับอักเสบบี", func(r models.LabResult) *models.LabResultFlag { return r.HBsAg }},
	{"hbsab", "HBsAb", models.LabResultPositive, "ภูมิคุ้มกันไวรัสตับอักเสบบีไม่เพียงพอ ควรพิจารณาฉีดวัคซีนกระตุ้น", func(r models.LabResult) *models.LabResultFlag { return r.HBsAb }},
	{"anti_hcv", "Anti HCV", models.LabResultNegative, "พบการติดเชื้อไวรัสตับอักเสบซี", func(r models.LabResult) *models.LabResultFlag { return r.AntiHCV }},
	{"anti_hiv", "Anti HIV", models.LabResultNegative, "พบการติดเชื้อเอชไอวี", func(r models.LabResult) *models.LabResultFlag { return r.AntiHIV }},
}

// LabFlagLabel returns the Thai label for a lab result flag pointer, or
// "-" if nil — exported so main.go can register it as a template func
// (same pattern as handler.DialysateAppearanceLabel for CAPD).
func LabFlagLabel(p *models.LabResultFlag) string {
	if p == nil {
		return "-"
	}
	switch *p {
	case models.LabResultNegative:
		return "ลบ (Negative)"
	case models.LabResultPositive:
		return "บวก (Positive)"
	default:
		return "-"
	}
}

// LabFlagShortLabel is LabFlagLabel without the English parenthetical —
// used in the /lab-results table where 30+ columns leaves little room per
// cell.
func LabFlagShortLabel(p *models.LabResultFlag) string {
	if p == nil {
		return "-"
	}
	switch *p {
	case models.LabResultNegative:
		return "ลบ"
	case models.LabResultPositive:
		return "บวก"
	default:
		return "-"
	}
}

// LabFlagSelected reports whether p equals the given raw enum value
// ("negative"/"positive") — used to pre-select a <select> option in
// lab_results_form.html without main.go's "deref" FuncMap entry needing a
// *models.LabResultFlag-specific case.
func LabFlagSelected(p *models.LabResultFlag, value string) bool {
	return p != nil && string(*p) == value
}

type labLatestNumeric struct {
	Value   float64
	LogDate time.Time
}

type labLatestEnum struct {
	Value   models.LabResultFlag
	LogDate time.Time
}

// labLatestValues holds, for one patient, the most recent non-null value
// of every lab field INDEPENDENTLY — not the most recent row, since
// different values are tested on different schedules (see
// models.LabResult's doc comment).
type labLatestValues struct {
	Numeric map[string]labLatestNumeric
	Enum    map[string]labLatestEnum
}

// resolveLatestLabValues expects rows ordered log_date DESC (ties broken
// by id DESC) — the first non-null value encountered per field is its
// most recent one.
func resolveLatestLabValues(rows []models.LabResult, isHD bool) labLatestValues {
	result := labLatestValues{Numeric: map[string]labLatestNumeric{}, Enum: map[string]labLatestEnum{}}

	fields := labNumericFields
	if isHD {
		fields = append(append([]labNumericField{}, labNumericFields...), labHDNumericFields...)
	}

	for _, row := range rows {
		for _, f := range fields {
			if _, found := result.Numeric[f.Key]; found {
				continue
			}
			if v := f.Get(row); v != nil {
				result.Numeric[f.Key] = labLatestNumeric{Value: *v, LogDate: row.LogDate}
			}
		}
		for _, f := range labEnumFields {
			if _, found := result.Enum[f.Key]; found {
				continue
			}
			if v := f.Get(row); v != nil {
				result.Enum[f.Key] = labLatestEnum{Value: *v, LogDate: row.LogDate}
			}
		}
	}
	return result
}

// LabAbnormalItem is one line of the rule-based abnormal-values list — see
// buildLabAbnormalItems for the "no synthesis across values" rule.
type LabAbnormalItem struct {
	Message string
	LogDate time.Time
}

func formatLabValue(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

// buildLabAbnormalItems is the rule-based (no AI, no cross-value
// interpretation) abnormal-values list for the dashboard summary card and
// /lab-results — see Phase 6 spec: each line reports exactly one value
// against its own reference range, never a synthesized judgment spanning
// multiple values (e.g. never "PTH+Ca+PO4 together suggest bone disease").
func buildLabAbnormalItems(latest labLatestValues, isHD bool) []LabAbnormalItem {
	var items []LabAbnormalItem

	fields := labNumericFields
	if isHD {
		fields = append(append([]labNumericField{}, labNumericFields...), labHDNumericFields...)
	}
	for _, f := range fields {
		lv, ok := latest.Numeric[f.Key]
		if !ok {
			continue
		}
		abnormal, direction := f.Range.Classify(lv.Value)
		if !abnormal {
			continue
		}
		directionText := "สูงกว่าเกณฑ์"
		if direction == "low" {
			directionText = "ต่ำกว่าเกณฑ์"
		}
		items = append(items, LabAbnormalItem{
			Message: fmt.Sprintf("%s %s: %s %s (ปกติ %s)", f.Label, directionText, formatLabValue(lv.Value), f.Unit, f.Range.String()),
			LogDate: lv.LogDate,
		})
	}

	for _, f := range labEnumFields {
		lv, ok := latest.Enum[f.Key]
		if !ok {
			continue
		}
		if lv.Value == f.NormalValue {
			continue
		}
		resultText := "ผลบวก"
		if lv.Value == models.LabResultNegative {
			resultText = "ผลลบ"
		}
		items = append(items, LabAbnormalItem{
			Message: fmt.Sprintf("%s %s (%s)", f.Label, resultText, f.AbnormalNote),
			LogDate: lv.LogDate,
		})
	}

	return items
}

// labSummaryData is the dashboard card's data source — reuses the exact
// same resolution/classification code as /lab-results so the two can
// never drift apart (same pattern as apdKPICards/capdKPICards/hdKPICards
// being shared between /apd|/capd|/hd and the dashboard).
func (h *AuthHandler) labSummaryData(profileID uint64, isHD bool) (items []LabAbnormalItem, hasData bool) {
	var rows []models.LabResult
	h.DB.Where("patient_profile_id = ?", profileID).Order("log_date DESC, id DESC").Find(&rows)
	if len(rows) == 0 {
		return nil, false
	}
	latest := resolveLatestLabValues(rows, isHD)
	return buildLabAbnormalItems(latest, isHD), true
}

// ---- GET /lab-results ----

func (h *AuthHandler) LabResultsList(c echo.Context) error {
	user, profile, err := h.requireLabResultsPatient(c)
	if user == nil {
		return err
	}
	isHD := isHDProfile(profile)

	var rows []models.LabResult
	h.DB.Where("patient_profile_id = ?", profile.ID).Order("log_date DESC, id DESC").Find(&rows)

	latest := resolveLatestLabValues(rows, isHD)
	abnormalItems := buildLabAbnormalItems(latest, isHD)

	var chronological []models.LabResult
	h.DB.Where("patient_profile_id = ?", profile.ID).Order("log_date ASC, id ASC").Find(&chronological)

	charts := map[string]interface{}{
		"hb": buildLabTrendSVG(chronological, "Hb", "g/dL", func(r models.LabResult) (float64, bool) {
			if r.Hb == nil {
				return 0, false
			}
			return *r.Hb, true
		}),
		"k": buildLabTrendSVG(chronological, "K", "mEq/L", func(r models.LabResult) (float64, bool) {
			if r.K == nil {
				return 0, false
			}
			return *r.K, true
		}),
		"albumin": buildLabTrendSVG(chronological, "Albumin", "g/dL", func(r models.LabResult) (float64, bool) {
			if r.Albumin == nil {
				return 0, false
			}
			return *r.Albumin, true
		}),
		"cr": buildLabTrendSVG(chronological, "Cr", "mg/dL", func(r models.LabResult) (float64, bool) {
			if r.Cr == nil {
				return 0, false
			}
			return *r.Cr, true
		}),
	}

	data := map[string]interface{}{
		"Rows":             rows,
		"AbnormalItems":    abnormalItems,
		"IsHD":             isHD,
		"Charts":           charts,
		"Disclaimer":       labrange.Disclaimer,
		"KtVReferenceText": labrange.KtVReferenceText,
	}
	return c.Render(http.StatusOK, "lab_results_list.html", withNav(data, user, h.navInfoFromProfile(user, profile), "/lab-results"))
}
