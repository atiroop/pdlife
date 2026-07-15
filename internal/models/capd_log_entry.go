package models

import "time"

type DialysateAppearance string

const (
	DialysateClear  DialysateAppearance = "clear"
	DialysateCloudy DialysateAppearance = "cloudy"
	DialysateBloody DialysateAppearance = "bloody"
)

// CapdLogEntry is one exchange cycle of a CAPD (Continuous Ambulatory
// Peritoneal Dialysis) log book record. Unlike APD, which logs one entry
// per patient per day, CAPD logs one row per cycle (typically 1-5 per
// day) — see docs/schema_spec.md.
type CapdLogEntry struct {
	ID                    uint64              `gorm:"column:id;primaryKey"`
	PatientProfileID      uint64              `gorm:"column:patient_profile_id;not null"`
	LogDate               time.Time           `gorm:"column:log_date;type:date;not null"`
	CycleNumber           int                 `gorm:"column:cycle_number;not null"`
	DextroseConcentration float64             `gorm:"column:dextrose_concentration;type:decimal(4,2);not null"`
	FillStartTime         string              `gorm:"column:fill_start_time;not null"`
	FillEndTime           string              `gorm:"column:fill_end_time;not null"`
	FillVolumeML          int                 `gorm:"column:fill_volume_ml;not null"`
	DrainStartTime        string              `gorm:"column:drain_start_time;not null"`
	DrainEndTime          string              `gorm:"column:drain_end_time;not null"`
	DrainVolumeML         int                 `gorm:"column:drain_volume_ml;not null"`
	UFVolumeML            int                 `gorm:"column:uf_volume_ml;not null"`
	DialysateAppearance   DialysateAppearance `gorm:"column:dialysate_appearance;type:enum('clear','cloudy','bloody');not null"`
	WeightKG              float64             `gorm:"column:weight_kg;type:decimal(5,2);not null"`
	BPSystolic            int                 `gorm:"column:bp_systolic;not null"`
	BPDiastolic           int                 `gorm:"column:bp_diastolic;not null"`
	UrineOutputML         *int                `gorm:"column:urine_output_ml"`
	CreatedAt             time.Time           `gorm:"column:created_at"`
	UpdatedAt             time.Time           `gorm:"column:updated_at"`
}

func (CapdLogEntry) TableName() string {
	return "capd_log_entries"
}

// ComputeUF sets UFVolumeML from the fill/drain volumes. Called before
// every insert/update — the field is never taken from user input directly.
func (e *CapdLogEntry) ComputeUF() {
	e.UFVolumeML = e.DrainVolumeML - e.FillVolumeML
}

// IsPeritonitisRisk reports whether this cycle's dialysate appearance
// should trigger the peritonitis alert banner.
func (e CapdLogEntry) IsPeritonitisRisk() bool {
	return e.DialysateAppearance == DialysateCloudy || e.DialysateAppearance == DialysateBloody
}
