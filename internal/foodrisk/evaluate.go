package foodrisk

import "fmt"

// Level is the visual category a Badge renders as. It intentionally
// separates "positive" and "info" from "good" even though they currently
// map to similar colors — see Level.CSSClass — because they mean different
// things (a nutrient being safely low vs. a food being a good protein
// source vs. a neutral fact like high water content).
type Level string

const (
	LevelGood     Level = "good"     // safely low (K/P/Na traffic light)
	LevelWatch    Level = "watch"    // moderate (K/P/Na traffic light)
	LevelAlert    Level = "alert"    // high, worth attention (K/P/Na traffic light)
	LevelPositive Level = "positive" // e.g. "good protein source"
	LevelInfo     Level = "info"     // neutral fact, e.g. high moisture
	LevelUnknown  Level = "unknown"  // missing or invalid data — never shown as safe
	LevelNone     Level = "none"     // no badge — caller should render the plain value, if any, with no pill
)

// CSSClass maps a Level to the pill-* classes defined in
// web/templates/_theme.html. LevelPositive reuses pill-good on purpose:
// both mean "this is the desirable state," just for different reasons.
func (l Level) CSSClass() string {
	switch l {
	case LevelGood, LevelPositive:
		return "pill-good"
	case LevelWatch:
		return "pill-watch"
	case LevelAlert:
		return "pill-alert"
	case LevelInfo:
		return "pill-info"
	case LevelUnknown:
		return "pill-unknown"
	default:
		return ""
	}
}

// Badge is what a template renders for one nutrient value. Detail carries
// the actual number so a color is never shown without the value behind it
// (see docs — "badge แสดงค่าจริงกำกับด้วยเสมอ"). Tooltip is only set for
// LevelUnknown, explaining that missing data is not the same as safe.
type Badge struct {
	Level   Level
	Text    string
	Detail  string
	Tooltip string
}

const unknownTooltip = "ยังไม่มีข้อมูลจากแหล่งต้นทาง ไม่ได้แปลว่าปลอดภัย"

// isUnknown treats a missing measurement (nil) and a physically impossible
// negative value the same way: never color it green/safe, always fall
// back to the neutral "no data" badge. A negative value here is the same
// class of data-quality problem the source system's nutrition_sanity.py
// flagged (docs/foodcheck_survey.md 4.1) — showing any colored risk badge
// for a value that's already known to be wrong would be worse than
// admitting there's no usable number.
func isUnknown(value *float64) bool {
	return value == nil || *value < 0
}

func unknownBadge(nutrientLabel string) Badge {
	return Badge{
		Level:   LevelUnknown,
		Text:    "ไม่มีข้อมูล",
		Tooltip: unknownTooltip,
	}
}

func evaluateTrafficLight(value *float64, t TrafficLightThresholds, label, unit string) Badge {
	if isUnknown(value) {
		return unknownBadge(label)
	}
	v := *value
	detail := fmt.Sprintf("%.0f %s/100g", v, unit)
	switch {
	case v > t.AlertAbove:
		return Badge{Level: LevelAlert, Text: label + "สูง", Detail: detail}
	case v < t.GoodBelow:
		return Badge{Level: LevelGood, Text: label + "ต่ำ", Detail: detail}
	default:
		return Badge{Level: LevelWatch, Text: label + "ปานกลาง", Detail: detail}
	}
}

// EvaluatePotassium classifies a Potassium value (mg per 100g).
func EvaluatePotassium(mgPer100g *float64) Badge {
	return evaluateTrafficLight(mgPer100g, PotassiumThresholds, "โพแทสเซียม", "mg")
}

// EvaluatePhosphorus classifies a Phosphorus value (mg per 100g).
func EvaluatePhosphorus(mgPer100g *float64) Badge {
	return evaluateTrafficLight(mgPer100g, PhosphorusThresholds, "ฟอสฟอรัส", "mg")
}

// EvaluateSodium classifies a Sodium value (mg per 100g).
func EvaluateSodium(mgPer100g *float64) Badge {
	return evaluateTrafficLight(mgPer100g, SodiumThresholds, "โซเดียม", "mg")
}

// EvaluateProtein returns a positive badge at/above ProteinGoodAtOrAbove,
// no badge below it (LevelNone), or the unknown badge for missing/invalid
// data.
func EvaluateProtein(gPer100g *float64) Badge {
	if isUnknown(gPer100g) {
		return unknownBadge("โปรตีน")
	}
	if *gPer100g >= ProteinGoodAtOrAbove {
		return Badge{
			Level:  LevelPositive,
			Text:   "แหล่งโปรตีนดี",
			Detail: fmt.Sprintf("%.1f g/100g", *gPer100g),
		}
	}
	return Badge{Level: LevelNone}
}

// EvaluateEnergy never returns a colored badge — energy needs depend on
// individual weight/BMI, not a per-food constant — but still distinguishes
// missing/invalid data (LevelUnknown) from a normal value (LevelNone,
// meaning "just show the number").
func EvaluateEnergy(kcalPer100g *float64) Badge {
	if isUnknown(kcalPer100g) {
		return unknownBadge("พลังงาน")
	}
	return Badge{Level: LevelNone}
}

// EvaluateMoisture returns an informational (not warning) tag at/above
// MoistureInfoAtOrAbove, no badge below it, or the unknown badge for
// missing/invalid data.
func EvaluateMoisture(gPer100g *float64) Badge {
	if isUnknown(gPer100g) {
		return unknownBadge("น้ำ/ความชื้น")
	}
	if *gPer100g >= MoistureInfoAtOrAbove {
		return Badge{
			Level:  LevelInfo,
			Text:   "น้ำสูง",
			Detail: "นับรวมในโควตาน้ำต่อวัน",
		}
	}
	return Badge{Level: LevelNone}
}

// Evaluate dispatches by the canonical nutrient name used throughout Food
// Check (foodcheck_pd_nutrients.nutrient_name / v_foodcheck_pd_nutrients),
// so callers iterating over that view's rows don't need their own switch.
// An unrecognized name returns LevelNone rather than panicking — this
// package only knows how to badge the 6 PD-critical nutrients.
func Evaluate(nutrientName string, value *float64) Badge {
	switch nutrientName {
	case "Potassium":
		return EvaluatePotassium(value)
	case "Phosphorus":
		return EvaluatePhosphorus(value)
	case "Sodium":
		return EvaluateSodium(value)
	case "Protein, total":
		return EvaluateProtein(value)
	case "Energy, by calculation":
		return EvaluateEnergy(value)
	case "Moisture":
		return EvaluateMoisture(value)
	default:
		return Badge{Level: LevelNone}
	}
}
