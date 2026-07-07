package kpi

// Ranges holds the clinical reference thresholds used to color KPI cards.
// PLACEHOLDER VALUES — pending confirmation from the medical team. Edit
// this struct (and redeploy) once real thresholds are confirmed; nothing
// else in the codebase needs to change.
var Ranges = struct {
	TotalUFML struct {
		WatchBelow float64 // below this = watch (but >= AlertBelow)
		WatchAbove float64 // above this = watch
		AlertBelow float64 // below this = alert (e.g. 0 = no UF at all)
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
}{
	TotalUFML: struct {
		WatchBelow float64
		WatchAbove float64
		AlertBelow float64
	}{
		// Brief specified "watch below 500" but "normal starts at 800",
		// leaving 500-799 undefined — treated as watch here until the
		// medical team confirms the actual boundary.
		WatchBelow: 800,
		WatchAbove: 2000,
		AlertBelow: 0,
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
}
