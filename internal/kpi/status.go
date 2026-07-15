// Package kpi computes clinical status (good / watch / alert) for the
// dashboard KPI cards from reference ranges. The thresholds have been
// reviewed and approved by the PD Clinic medical team — see
// docs/clinical_review.md and reference_ranges.go for the values and how
// to change them.
package kpi

type Status string

const (
	StatusGood  Status = "good"
	StatusWatch Status = "watch"
	StatusAlert Status = "alert"
)

// Label returns the Thai pill label shown on a KPI card.
func (s Status) Label() string {
	switch s {
	case StatusGood:
		return "ปกติ"
	case StatusWatch:
		return "เฝ้าระวัง"
	case StatusAlert:
		return "ผิดปกติ"
	default:
		return ""
	}
}

// TotalUF classifies a day's total ultrafiltration volume (ml).
func TotalUF(ml float64) Status {
	r := Ranges.TotalUFML
	switch {
	case ml < r.AlertBelow:
		return StatusAlert
	case ml < r.WatchBelow, ml > r.WatchAbove:
		return StatusWatch
	default:
		return StatusGood
	}
}

// CapdDailyUF classifies a day's net CAPD ultrafiltration (sum of
// uf_volume_ml across that day's cycles).
func CapdDailyUF(ml float64) Status {
	r := Ranges.CapdDailyUFML
	switch {
	case ml <= r.AlertAtOrBelow:
		return StatusAlert
	case ml < r.WatchBelow, ml > r.WatchAbove:
		return StatusWatch
	default:
		return StatusGood
	}
}

// WeightChange classifies today's weight against the previous 7-day average.
// deltaKg is today's weight minus that average (can be negative).
func WeightChange(deltaKg float64) Status {
	r := Ranges.WeightChangeKG
	abs := deltaKg
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs > r.AlertAbs:
		return StatusAlert
	case abs > r.WatchAbs:
		return StatusWatch
	default:
		return StatusGood
	}
}

// BloodPressure classifies a systolic/diastolic reading. The worse of the
// two sub-scores determines the overall status.
func BloodPressure(systolic, diastolic int) Status {
	r := Ranges.BloodPressure
	sys := classifyBP(systolic, r.NormalMaxSystolic, r.WatchMaxSystolic)
	dia := classifyBP(diastolic, r.NormalMaxDiastolic, r.WatchMaxDiastolic)
	return worse(sys, dia)
}

// classifyBP: good if v < normalMax, watch if normalMax <= v <= watchMax,
// alert if v > watchMax.
func classifyBP(v, normalMax, watchMax int) Status {
	switch {
	case v > watchMax:
		return StatusAlert
	case v >= normalMax:
		return StatusWatch
	default:
		return StatusGood
	}
}

// HDPostVsDry classifies post-dialysis weight against dry weight.
// deltaKg is post-dialysis weight minus dry weight (can be negative).
func HDPostVsDry(deltaKg float64) Status {
	r := Ranges.HDPostVsDryKG
	switch {
	case deltaKg > r.WatchOverMaxKG:
		return StatusAlert
	case deltaKg > r.NormalAbsKG:
		return StatusWatch
	case deltaKg < -r.WatchUnderMaxKG:
		return StatusAlert
	case deltaKg < -r.NormalAbsKG:
		return StatusWatch
	default:
		return StatusGood
	}
}

// HDInterdialyticGain classifies weight gained since the previous HD
// session (this session's pre-dialysis weight minus the previous
// session's post-dialysis weight). Negative gain (weight loss) is
// treated as good — the brief gives no lower-bound threshold.
func HDInterdialyticGain(gainKg float64) Status {
	r := Ranges.HDInterdialyticGainKG
	switch {
	case gainKg > r.WatchMaxKG:
		return StatusAlert
	case gainKg > r.NormalMaxKG:
		return StatusWatch
	default:
		return StatusGood
	}
}

// HDPostDialysisBP classifies post-dialysis systolic BP — the
// HD-specific intradialytic-hypotension risk signal (distinct from the
// shared pre-dialysis BloodPressure() thresholds).
func HDPostDialysisBP(systolic int) Status {
	r := Ranges.HDPostBPSystolic
	switch {
	case systolic < r.WatchMinSystolic:
		return StatusAlert
	case systolic < r.NormalMinSystolic:
		return StatusWatch
	default:
		return StatusGood
	}
}

func worse(a, b Status) Status {
	rank := map[Status]int{StatusGood: 0, StatusWatch: 1, StatusAlert: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
