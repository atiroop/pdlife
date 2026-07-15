// Package foodrisk computes risk-indicator badges for the 6 PD-critical
// nutrients tracked by Food Check (see foodcheck_pd_nutrients /
// docs/foodcheck_survey.md). This mirrors internal/kpi and
// internal/labrange's posture: thresholds kept in one place so they can be
// edited later without touching any calling code.
//
// The source system (foodcheck.jocky.website) never shipped numeric
// thresholds at all — it only had a `risk_direction` ('high'/'low') flag
// that no code ever read (see docs/foodcheck_survey.md 4.1). This
// package's thresholds were subsequently reviewed and approved by the PD
// Clinic medical team (see docs/clinical_review.md). One shared set works
// across both INMU and Anamai despite their materially different value
// distributions, per the 2026-07-08 stats query against the migrated data
// (Anamai's P25 sodium is 1mg/100g vs INMU's 28mg/100g, for example).
package foodrisk

// Disclaimer must accompany every place a badge from this package is
// displayed — see docs/foodcheck_survey.md risk #2 (the source system's
// own hard rule against implying medical judgment from a data-quality
// warning; this package makes an even stronger claim, a risk *indicator*,
// so the disclaimer matters even more here).
const Disclaimer = "เกณฑ์อ้างอิงนี้ผ่านการตรวจสอบและอนุมัติจากทีมแพทย์ผู้ดูแลผู้ป่วยโรคไตแล้ว อย่างไรก็ตาม ยังคงเป็นข้อมูลทั่วไปเพื่อการติดตามเบื้องต้นเท่านั้น ไม่ใช่การวินิจฉัยเฉพาะบุคคล กรุณาปรึกษาแพทย์ผู้ดูแลท่านโดยตรงเสมอ"

// TrafficLightThresholds defines the 3-tier boundaries for a nutrient
// where a higher value is worse: green below GoodBelow, red above
// AlertAbove, yellow for everything in between (the yellow band includes
// both boundary values themselves).
type TrafficLightThresholds struct {
	GoodBelow  float64
	AlertAbove float64
}

// Potassium/Phosphorus/Sodium thresholds, mg per 100g. Reviewed and
// approved by the PD Clinic medical team (see Disclaimer above and
// docs/clinical_review.md).
var (
	PotassiumThresholds  = TrafficLightThresholds{GoodBelow: 200, AlertAbove: 350}
	PhosphorusThresholds = TrafficLightThresholds{GoodBelow: 100, AlertAbove: 200}
	SodiumThresholds     = TrafficLightThresholds{GoodBelow: 120, AlertAbove: 400}
)

// ProteinGoodAtOrAbove: protein at or above this (g per 100g) earns a
// positive "good protein source" badge. Below it, no badge at all —
// coloring every food under 12g protein as a warning would flag nearly
// every vegetable and fruit, which is backwards (low protein isn't a
// hazard the way high potassium/phosphorus/sodium is).
const ProteinGoodAtOrAbove = 12.0

// MoistureInfoAtOrAbove: moisture at or above this (g per 100g) shows an
// informational tag about counting toward daily fluid intake. Never
// colored red/yellow — this is a fact about the food, not a warning.
const MoistureInfoAtOrAbove = 70.0
