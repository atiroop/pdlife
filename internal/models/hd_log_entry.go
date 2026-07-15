package models

import (
	"math"
	"time"
)

// HdLogEntry is one HD (Hemodialysis) session log book record — one row
// per session (typically 2-3 per week), matching ApdLogEntry's
// one-row-per-day shape rather than CapdLogEntry's per-cycle shape. See
// docs/schema_spec.md.
type HdLogEntry struct {
	ID                      uint64    `gorm:"column:id;primaryKey"`
	PatientProfileID        uint64    `gorm:"column:patient_profile_id;not null"`
	LogDate                 time.Time `gorm:"column:log_date;type:date;not null"`
	DryWeightKG             float64   `gorm:"column:dry_weight_kg;type:decimal(5,2);not null"`
	PreDialysisWeightKG     float64   `gorm:"column:pre_dialysis_weight_kg;type:decimal(5,2);not null"`
	PostDialysisWeightKG    float64   `gorm:"column:post_dialysis_weight_kg;type:decimal(5,2);not null"`
	PreDialysisBPSystolic   int       `gorm:"column:pre_dialysis_bp_systolic;not null"`
	PreDialysisBPDiastolic  int       `gorm:"column:pre_dialysis_bp_diastolic;not null"`
	PostDialysisBPSystolic  int       `gorm:"column:post_dialysis_bp_systolic;not null"`
	PostDialysisBPDiastolic int       `gorm:"column:post_dialysis_bp_diastolic;not null"`
	UFRemovedML             int       `gorm:"column:uf_removed_ml;not null"`
	Notes                   *string   `gorm:"column:notes"`
	CreatedAt               time.Time `gorm:"column:created_at"`
	UpdatedAt               time.Time `gorm:"column:updated_at"`
}

func (HdLogEntry) TableName() string {
	return "hd_log_entries"
}

// ComputeUFRemoved sets UFRemovedML from the pre/post dialysis weight.
// Called before every insert/update — this is a TOTAL volume removed
// during the session, not an hourly ultrafiltration rate, and is never
// taken from user input directly. math.Round (not truncation) avoids an
// off-by-one from float64 subtraction error (e.g. 67.0-65.2 == 1.7999...
// in IEEE 754, which int() truncates to 1799 instead of 1800).
func (e *HdLogEntry) ComputeUFRemoved() {
	e.UFRemovedML = int(math.Round((e.PreDialysisWeightKG - e.PostDialysisWeightKG) * 1000))
}
