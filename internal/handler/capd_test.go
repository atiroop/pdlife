package handler

import (
	"testing"
	"time"

	"github.com/atiroop/pdlife/internal/models"
)

func capdDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse date %q: %v", s, err)
	}
	return d
}

// TestAggregateCapdDaily_NetUFAndLastCycleWins mirrors the scenario in
// docs/schema_spec.md's manual test plan: 3-4 cycles logged in one day
// (e.g. 16/6/69) should roll up into one day's net UF (sum) and a
// "representative" weight/BP/urine taken from the last cycle of that day.
func TestAggregateCapdDaily_NetUFAndLastCycleWins(t *testing.T) {
	day := capdDate(t, "2026-06-16")
	urine := 300
	entries := []models.CapdLogEntry{
		{LogDate: day, CycleNumber: 1, FillVolumeML: 2000, DrainVolumeML: 2300, UFVolumeML: 300, WeightKG: 60.5, BPSystolic: 120, BPDiastolic: 80},
		{LogDate: day, CycleNumber: 2, FillVolumeML: 2000, DrainVolumeML: 2200, UFVolumeML: 200, WeightKG: 60.3, BPSystolic: 122, BPDiastolic: 82},
		{LogDate: day, CycleNumber: 3, FillVolumeML: 2000, DrainVolumeML: 2100, UFVolumeML: 100, WeightKG: 60.1, BPSystolic: 125, BPDiastolic: 84, UrineOutputML: &urine},
	}

	days := aggregateCapdDaily(entries)
	if len(days) != 1 {
		t.Fatalf("len(days) = %d, want 1", len(days))
	}
	got := days[0]

	if want := 300 + 200 + 100; got.UFTotal != want {
		t.Errorf("UFTotal = %d, want %d", got.UFTotal, want)
	}
	if got.LastWeight != 60.1 {
		t.Errorf("LastWeight = %v, want 60.1 (from cycle 3, the last cycle of the day)", got.LastWeight)
	}
	if got.LastBPSys != 125 || got.LastBPDia != 84 {
		t.Errorf("LastBP = %d/%d, want 125/84 (from cycle 3)", got.LastBPSys, got.LastBPDia)
	}
	if got.LastUrineML == nil || *got.LastUrineML != 300 {
		t.Errorf("LastUrineML = %v, want 300", got.LastUrineML)
	}
	if got.CycleCount != 3 {
		t.Errorf("CycleCount = %d, want 3", got.CycleCount)
	}
}

// TestAggregateCapdDaily_SplitsByDay confirms multi-day input produces one
// aggregate per calendar day, in the order the days first appear.
func TestAggregateCapdDaily_SplitsByDay(t *testing.T) {
	day1 := capdDate(t, "2026-06-15")
	day2 := capdDate(t, "2026-06-16")
	entries := []models.CapdLogEntry{
		{LogDate: day1, CycleNumber: 1, UFVolumeML: 100},
		{LogDate: day1, CycleNumber: 2, UFVolumeML: 150},
		{LogDate: day2, CycleNumber: 1, UFVolumeML: 200},
	}

	days := aggregateCapdDaily(entries)
	if len(days) != 2 {
		t.Fatalf("len(days) = %d, want 2", len(days))
	}
	if days[0].UFTotal != 250 {
		t.Errorf("day1 UFTotal = %d, want 250", days[0].UFTotal)
	}
	if days[1].UFTotal != 200 {
		t.Errorf("day2 UFTotal = %d, want 200", days[1].UFTotal)
	}
}
