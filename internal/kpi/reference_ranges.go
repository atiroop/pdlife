package kpi

// Disclaimer must accompany every place a Status/StatusLabel from this
// package is displayed (APD/CAPD/HD KPI cards) — see docs/clinical_review.md
// for the medical team's sign-off record.
const Disclaimer = "เกณฑ์อ้างอิงนี้ผ่านการตรวจสอบและอนุมัติจากทีมแพทย์ผู้ดูแลผู้ป่วยโรคไตแล้ว อย่างไรก็ตาม ยังคงเป็นข้อมูลทั่วไปเพื่อการติดตามเบื้องต้นเท่านั้น ไม่ใช่การวินิจฉัยเฉพาะบุคคล กรุณาปรึกษาแพทย์ผู้ดูแลท่านโดยตรงเสมอ"

// Ranges holds the clinical reference thresholds used to color KPI cards.
// Reviewed and approved by the PD Clinic medical team — see
// docs/clinical_review.md. Edit this struct (and redeploy) if a threshold
// is revised; nothing else in the codebase needs to change.
var Ranges = struct {
	TotalUFML struct {
		WatchBelow float64 // below this = watch (but >= AlertBelow)
		WatchAbove float64 // above this = watch
		AlertBelow float64 // below this = alert (e.g. 0 = no UF at all)
	}
	CapdDailyUFML struct {
		WatchBelow     float64 // below this = watch (but > AlertAtOrBelow)
		WatchAbove     float64 // above this = watch
		AlertAtOrBelow float64 // at or below this = alert
	}
	WeightChangeKG struct {
		WatchAbs float64 // |delta from 7-day avg| above this = watch
		AlertAbs float64 // |delta from 7-day avg| above this = alert
	}
	BloodPressure struct {
		NormalMaxSystolic  int // < this = normal
		WatchMaxSystolic   int // normal..this = watch, above = alert
		NormalMaxDiastolic int
		WatchMaxDiastolic  int
	}
	// HDPostVsDryKG classifies post-dialysis weight against dry weight
	// (delta = post - dry, can be negative). Deliberately asymmetric:
	// staying a bit over dry weight is watched more leniently than
	// dropping under it (risk of over-ultrafiltration).
	HDPostVsDryKG struct {
		NormalAbsKG     float64 // |delta| <= this = normal
		WatchOverMaxKG  float64 // delta in (NormalAbsKG, this] = watch (fluid still excess); above = alert
		WatchUnderMaxKG float64 // delta in [-this, -NormalAbsKG) = watch (over-UF risk); below -this = alert
	}
	// HDInterdialyticGainKG classifies weight gained between sessions
	// (this session's pre-dialysis weight minus the previous session's
	// post-dialysis weight).
	HDInterdialyticGainKG struct {
		NormalMaxKG float64 // <= this = normal
		WatchMaxKG  float64 // NormalMaxKG..this = watch; above = alert
	}
	// HDPostBPSystolic classifies post-dialysis systolic BP — HD-specific
	// intradialytic-hypotension risk signal, unlike the shared
	// pre-dialysis BloodPressure() thresholds above.
	HDPostBPSystolic struct {
		WatchMinSystolic  int // below this = alert (hypotension risk)
		NormalMinSystolic int // WatchMinSystolic..this-1 = watch; >= this = normal
	}
}{
	TotalUFML: struct {
		WatchBelow float64
		WatchAbove float64
		AlertBelow float64
	}{
		// Brief specified "watch below 500" but "normal starts at 800",
		// leaving 500-799 undefined — treated as watch here; confirmed by
		// the PD Clinic medical team (see docs/clinical_review.md).
		WatchBelow: 800,
		WatchAbove: 2000,
		AlertBelow: 0,
	},
	CapdDailyUFML: struct {
		WatchBelow     float64
		WatchAbove     float64
		AlertAtOrBelow float64
	}{
		// Brief specified normal 400-1500, watch <200 or >1500, alert <=0,
		// leaving 200-399 undefined — treated as watch (same gap-handling as
		// APD above); confirmed by the PD Clinic medical team (see
		// docs/clinical_review.md).
		WatchBelow:     400,
		WatchAbove:     1500,
		AlertAtOrBelow: 0,
	},
	WeightChangeKG: struct {
		WatchAbs float64
		AlertAbs float64
	}{
		WatchAbs: 1,
		AlertAbs: 2,
	},
	BloodPressure: struct {
		NormalMaxSystolic  int
		WatchMaxSystolic   int
		NormalMaxDiastolic int
		WatchMaxDiastolic  int
	}{
		NormalMaxSystolic:  130,
		WatchMaxSystolic:   160,
		NormalMaxDiastolic: 80,
		WatchMaxDiastolic:  100,
	},
	// Brief: normal +/-0.5kg from dry weight; watch 0.5-1.5kg over (fluid
	// excess remains) or >0.5kg under (over-UF risk); alert >1.5kg over or
	// >1kg under.
	HDPostVsDryKG: struct {
		NormalAbsKG     float64
		WatchOverMaxKG  float64
		WatchUnderMaxKG float64
	}{
		NormalAbsKG:     0.5,
		WatchOverMaxKG:  1.5,
		WatchUnderMaxKG: 1.0,
	},
	// Brief: normal <=2kg, watch 2-3kg, alert >3kg.
	HDInterdialyticGainKG: struct {
		NormalMaxKG float64
		WatchMaxKG  float64
	}{
		NormalMaxKG: 2,
		WatchMaxKG:  3,
	},
	// Brief: normal systolic >=100, watch 90-100, alert <90 (intradialytic
	// hypotension risk).
	HDPostBPSystolic: struct {
		WatchMinSystolic  int
		NormalMinSystolic int
	}{
		WatchMinSystolic:  90,
		NormalMinSystolic: 100,
	},
}
