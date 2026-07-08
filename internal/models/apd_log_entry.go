package models

import "time"

// ApdLogEntry is one day's APD (Automated Peritoneal Dialysis) log book
// record. Field set mirrors the legacy source system exactly (see
// docs/schema_spec.md) — one entry per patient per calendar day.
type ApdLogEntry struct {
	ID                 uint64    `gorm:"column:id;primaryKey"`
	PatientProfileID   uint64    `gorm:"column:patient_profile_id;not null"`
	EntryDate          time.Time `gorm:"column:entry_date;type:date;not null"`
	TreatmentStartTime string    `gorm:"column:treatment_start_time;not null"`
	WeightKG           float64   `gorm:"column:weight_kg;type:decimal(5,2);not null"`
	BPSystolic         int       `gorm:"column:bp_systolic;not null"`
	BPDiastolic        int       `gorm:"column:bp_diastolic;not null"`
	Pulse              int       `gorm:"column:pulse;not null"`
	BloodGlucoseMgDL   *int      `gorm:"column:blood_glucose_mg_dl"`
	IDrainVolumeML     int       `gorm:"column:i_drain_volume_ml;not null"`
	TotalUFML          int       `gorm:"column:total_uf_ml;not null"`
	UrineAvgDayML      int       `gorm:"column:urine_avg_day_ml;not null"`
	DrainageAppearance *string   `gorm:"column:drainage_appearance"`
	Remark             *string   `gorm:"column:remark"`
	PrescriptionID     *uint64   `gorm:"column:prescription_id"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (ApdLogEntry) TableName() string {
	return "apd_log_entries"
}
