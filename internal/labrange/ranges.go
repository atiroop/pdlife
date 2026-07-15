// Package labrange holds the reference ranges used to flag abnormal lab
// values on /lab-results and the dashboard summary card. Every threshold
// here is copied verbatim from the PD Clinic's own lab-tracking form, and
// has been reviewed and approved by the PD Clinic medical team — see
// docs/clinical_review.md and Disclaimer, which must accompany every
// place a range from this package is shown.
package labrange

import (
	"strconv"
	"strings"
)

// Disclaimer must accompany every place a range or classification from
// this package is displayed.
const Disclaimer = "เกณฑ์อ้างอิงนี้ผ่านการตรวจสอบและอนุมัติจากทีมแพทย์ผู้ดูแลผู้ป่วยโรคไตแล้ว อย่างไรก็ตาม ยังคงเป็นข้อมูลทั่วไปเพื่อการติดตามเบื้องต้นเท่านั้น ไม่ใช่การวินิจฉัยเฉพาะบุคคล กรุณาปรึกษาแพทย์ผู้ดูแลท่านโดยตรงเสมอ"

// KtVReferenceText is shown next to the Kt/V value on /lab-results —
// Kt/V is deliberately never auto-classified as normal/abnormal (the
// target depends on the patient's own dialysis schedule), so this is
// plain reference text, not a Range.
const KtVReferenceText = "เป้าหมาย 1.8 หากฟอกเลือด 2 ครั้ง/สัปดาห์ หรือ 1.2 หากฟอกเลือด 3 ครั้ง/สัปดาห์ — กรุณาเทียบกับตารางฟอกเลือดของท่านเอง หรือปรึกษาแพทย์"

// Range is a normal reference range with independently inclusive or
// exclusive bounds, since the source form mixes "70-110" (inclusive),
// ">65" (exclusive minimum), and ">=200" (inclusive minimum) style
// thresholds. A nil Min or Max means unbounded on that side.
type Range struct {
	Min          *float64
	MinInclusive bool
	Max          *float64
	MaxInclusive bool
}

func between(min, max float64) Range {
	return Range{Min: &min, MinInclusive: true, Max: &max, MaxInclusive: true}
}

func minInclusive(min float64) Range {
	return Range{Min: &min, MinInclusive: true}
}

func minExclusive(min float64) Range {
	return Range{Min: &min, MinInclusive: false}
}

func maxExclusive(max float64) Range {
	return Range{Max: &max, MaxInclusive: false}
}

// IsNormal reports whether v falls within the range.
func (r Range) IsNormal(v float64) bool {
	abnormal, _ := r.Classify(v)
	return !abnormal
}

// Classify reports whether v is abnormal, and if so, which direction it
// missed the range in ("high" or "low") — used to pick the "สูงกว่าเกณฑ์"
// vs "ต่ำกว่าเกณฑ์" wording on the abnormal-values list.
func (r Range) Classify(v float64) (abnormal bool, direction string) {
	if r.Min != nil {
		low := v < *r.Min
		if !r.MinInclusive {
			low = v <= *r.Min
		}
		if low {
			return true, "low"
		}
	}
	if r.Max != nil {
		high := v > *r.Max
		if !r.MaxInclusive {
			high = v >= *r.Max
		}
		if high {
			return true, "high"
		}
	}
	return false, ""
}

// String renders the range for display, e.g. "33-36", ">65", ">=200", "<7".
func (r Range) String() string {
	fmtNum := func(v float64) string {
		s := strconv.FormatFloat(v, 'f', 2, 64)
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
		return s
	}
	switch {
	case r.Min != nil && r.Max != nil:
		return fmtNum(*r.Min) + "-" + fmtNum(*r.Max)
	case r.Min != nil && r.MinInclusive:
		return ">=" + fmtNum(*r.Min)
	case r.Min != nil:
		return ">" + fmtNum(*r.Min)
	case r.Max != nil && r.MaxInclusive:
		return "<=" + fmtNum(*r.Max)
	case r.Max != nil:
		return "<" + fmtNum(*r.Max)
	default:
		return ""
	}
}

// Reference ranges — copied verbatim from the source lab-tracking form
// (see the Phase 6 spec / docs/schema_spec.md). DRAFT, see Disclaimer.
var (
	// ตรวจทุก 3 เดือน
	HctRange      = between(33, 36)
	HbRange       = between(10, 12)
	WBCRange      = between(4500, 10000)
	PlateletRange = between(100000, 500000)
	BUNRange      = between(7, 21)
	CrRange       = between(0.73, 1.18)
	NaRange       = between(136, 145)
	KRange        = between(3.5, 5.5)
	CO2Range      = between(22, 29)
	CaRange       = between(8.5, 10.5)
	PO4Range      = between(2.5, 4.5)
	AlbuminRange  = between(3.5, 4.0)
	URRRange      = minExclusive(65)  // >65, HD only
	NPCRRange     = minExclusive(1.2) // >1.2, HD only

	// ตรวจทุก 6 เดือน / 1 ปี
	FBSRange       = between(70, 110)
	HbA1CRange     = maxExclusive(7) // <7
	UricAcidRange  = between(3, 7)
	PTHRange       = between(150, 300)
	FerritinRange  = minInclusive(200) // >=200, single threshold (no upper bound given)
	SerumIronRange = between(37, 145)
	TIBCRange      = between(228, 428)
	TSatRange      = minExclusive(30)  // >30
	CholRange      = maxExclusive(200) // <200
	HDLRange       = between(40, 50)
	LDLRange       = maxExclusive(130) // <130
)
