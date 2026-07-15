// Package foodunit converts Food Check nutrient values (stored per 100 g)
// into the amount a patient actually eats, in whichever serving unit they
// pick. This is a straight port of the source system's
// web/unit_conversion.py (foodcheck.jocky.website) — see
// docs/foodcheck_survey.md 4.3 — kept as a literal port rather than a
// redesign so the numbers match the original exactly (verified against
// live foodcheck.jocky.website during development).
package foodunit

import (
	"math"
	"strings"
)

// Mass units convert directly; volume units need a food's density (g per
// mL) to convert to grams — see FoodInfo/ResolveDensity below.
const (
	TeaspoonML   = 5.0
	TablespoonML = 15.0
	CupML        = 240.0
	OzToG        = 28.3495
)

// fallbackRule is one entry of the source's FALLBACK_DENSITIES list — used
// only when a food has no explicit "Density" nutrient value.
type fallbackRule struct {
	Category      string
	Keywords      []string
	DensityGPerML float64
	MatchMode     string // "exact_start" | "contains" | "condiment"
	Note          string
}

// fallbackDensities is copied verbatim (keywords, densities, match modes,
// notes) from web/unit_conversion.py's FALLBACK_DENSITIES on
// foodcheck.jocky.website — do not add/remove/reorder without re-checking
// the source, since this list is the whole point of the port.
var fallbackDensities = []fallbackRule{
	{
		Category:      "table_salt",
		Keywords:      []string{"เกลือ", "salt"},
		DensityGPerML: 1.2,
		MatchMode:     "exact_start",
		Note:          "ค่าประมาณทั่วไปสำหรับเกลือป่น; ความละเอียดและวิธีตักมีผลต่อกรัมจริง",
	},
	{
		Category:      "granulated_sugar",
		Keywords:      []string{"น้ำตาลทราย", "sugar cane, brown", "sugar, granulated"},
		DensityGPerML: 0.8,
		MatchMode:     "contains",
		Note:          "ค่าประมาณทั่วไปสำหรับน้ำตาลทราย; ชนิดน้ำตาลและวิธีตักมีผลต่อกรัมจริง",
	},
	{
		Category:      "sauce_without_density",
		Keywords:      []string{"ซอส", "sauce", "น้ำปลา", "fish sauce", "ซีอิ๊ว", "soy sauce"},
		DensityGPerML: 1.0,
		MatchMode:     "condiment",
		Note:          "ค่าประมาณทั่วไปเมื่อไม่มี density ในฐานข้อมูล; ซอสแต่ละชนิดอาจหนักต่างกัน",
	},
}

// Unit is one entry of the source's STANDARD_UNITS list.
type Unit struct {
	Code   string
	Label  string
	Kind   string  // "mass" | "volume"
	Factor float64 // mass units only: grams per unit
	ML     float64 // volume units only: mL per unit
}

// StandardUnits mirrors STANDARD_UNITS from web/unit_conversion.py exactly
// (order, codes, labels, factors).
var StandardUnits = []Unit{
	{Code: "g", Label: "กรัม (g)", Kind: "mass", Factor: 1},
	{Code: "kg", Label: "กิโลกรัม (kg)", Kind: "mass", Factor: 1000},
	{Code: "oz", Label: "ออนซ์ (oz)", Kind: "mass", Factor: OzToG},
	{Code: "ml", Label: "มิลลิลิตร (mL)", Kind: "volume", ML: 1},
	{Code: "l", Label: "ลิตร (L)", Kind: "volume", ML: 1000},
	{Code: "tsp", Label: "ช้อนชา (tsp)", Kind: "volume", ML: TeaspoonML},
	{Code: "tbsp", Label: "ช้อนโต๊ะ (tbsp)", Kind: "volume", ML: TablespoonML},
	{Code: "cup", Label: "ถ้วยตวง (cup, 240 mL)", Kind: "volume", ML: CupML},
}

// FoodInfo carries the identifying fields ResolveDensity's keyword
// matching needs — mirrors unit_conversion.py's _food_text() inputs
// (name_th, name_en, scientific_name, group_name, food_code) plus status
// (used only by the "condiment" match mode, see docs/foodcheck_survey.md).
// For Anamai foods, Status should be food_type — same field the source's
// food_dict maps it from, even though Anamai's food_type taxonomy doesn't
// actually share INMU's 'N' = "condiments" code (a quirk of the original
// design, kept as-is rather than "fixed" since this is a literal port).
type FoodInfo struct {
	NameTh         string
	NameEn         string
	ScientificName string
	GroupName      string
	FoodCode       string
	Status         string
}

func foodText(f FoodInfo) string {
	parts := []string{f.NameTh, f.NameEn, f.ScientificName, f.GroupName, f.FoodCode}
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.ToLower(strings.Join(nonEmpty, " "))
}

func matchesFallback(f FoodInfo, rule fallbackRule) bool {
	switch rule.MatchMode {
	case "exact_start":
		nameTh := strings.ToLower(strings.TrimSpace(f.NameTh))
		nameEn := strings.ToLower(strings.TrimSpace(f.NameEn))
		for _, kw := range rule.Keywords {
			if strings.HasPrefix(nameTh, kw) || strings.HasPrefix(nameEn, kw) {
				return true
			}
		}
		return false
	case "condiment":
		if strings.ToUpper(strings.TrimSpace(f.Status)) != "N" {
			return false
		}
		text := foodText(f)
		for _, kw := range rule.Keywords {
			if strings.Contains(text, kw) {
				return true
			}
		}
		return false
	default: // "contains"
		text := foodText(f)
		for _, kw := range rule.Keywords {
			if strings.Contains(text, kw) {
				return true
			}
		}
		return false
	}
}

// DensityResult mirrors resolve_density()'s (density, source_key, note)
// return — IsFallback distinguishes a measured value (Source == "density")
// from a keyword-matched estimate (Source == one of the fallback
// categories), for the UI's "ค่าประมาณ" transparency indicator.
type DensityResult struct {
	GPerML     *float64
	Source     string
	Note       string
	IsFallback bool
}

// ResolveDensity finds a food's density in g/mL: first the food's own
// "Density" nutrient value (densityValue, from foodcheck_food_nutrients —
// Anamai foods never have one, see docs/foodcheck_survey.md), then the
// keyword-based fallback rules, in that order. A zero-value DensityResult
// (GPerML == nil) means density is unresolvable for this food.
func ResolveDensity(f FoodInfo, densityValue *float64) DensityResult {
	if densityValue != nil && *densityValue > 0 {
		return DensityResult{
			GPerML: densityValue,
			Source: "density",
			Note:   "คำนวณจาก density ในฐานข้อมูล (กรัมต่อมิลลิลิตร)",
		}
	}
	for _, rule := range fallbackDensities {
		if matchesFallback(f, rule) {
			v := rule.DensityGPerML
			return DensityResult{
				GPerML:     &v,
				Source:     rule.Category,
				Note:       rule.Note,
				IsFallback: true,
			}
		}
	}
	return DensityResult{}
}

// UnitConversion is one entry of GetUnitConversions' Units list — grams
// per unit for one standard unit, or unavailable (GramsPerUnit == nil)
// when it's a volume unit and density is unresolvable.
type UnitConversion struct {
	Code         string
	Label        string
	GramsPerUnit *float64
	Available    bool
}

// Conversions is the full grams-per-unit table for one food, mirroring
// get_unit_conversions()'s return shape.
type Conversions struct {
	DensityAvailable bool
	DensitySource    string
	Note             string
	IsFallback       bool
	Units            []UnitConversion
}

// GetUnitConversions returns grams-per-unit for every standard unit. Mass
// units (g/kg/oz) are always available; volume units (mL/L/tsp/tbsp/cup)
// only when ResolveDensity finds a density for this food.
func GetUnitConversions(f FoodInfo, densityValue *float64) Conversions {
	density := ResolveDensity(f, densityValue)

	units := make([]UnitConversion, 0, len(StandardUnits))
	for _, u := range StandardUnits {
		var grams *float64
		if u.Kind == "mass" {
			v := u.Factor
			grams = &v
		} else if density.GPerML != nil {
			v := math.Round(*density.GPerML*u.ML*1e6) / 1e6
			grams = &v
		}
		units = append(units, UnitConversion{
			Code:         u.Code,
			Label:        u.Label,
			GramsPerUnit: grams,
			Available:    grams != nil,
		})
	}

	return Conversions{
		DensityAvailable: density.GPerML != nil,
		DensitySource:    density.Source,
		Note:             density.Note,
		IsFallback:       density.IsFallback,
		Units:            units,
	}
}

// GramsFor converts amount+unitCode into grams using conv, or nil if
// unitCode is unknown or unavailable for this food (never trust a
// caller-supplied unit code without this check — see internal/handler's
// FoodCheckNutrition, which rejects the request outright in that case).
func GramsFor(amount float64, unitCode string, conv Conversions) *float64 {
	for _, u := range conv.Units {
		if u.Code != unitCode {
			continue
		}
		if !u.Available || u.GramsPerUnit == nil {
			return nil
		}
		v := amount * *u.GramsPerUnit
		return &v
	}
	return nil
}

// ScaleValue converts a per-100g nutrient value to the amount present in
// gramsAmount grams of food.
func ScaleValue(per100g, gramsAmount float64) float64 {
	return per100g * gramsAmount / 100
}
