package models

import "time"

// ApdPrescription is a treatment prescription profile for an APD patient
// (dialysis solution bags, cycle counts, dwell time, etc). A daily log
// entry references the prescription in effect at the time it was recorded.
type ApdPrescription struct {
	ID                 uint64    `gorm:"column:id;primaryKey"`
	PatientProfileID   uint64    `gorm:"column:patient_profile_id;not null"`
	Name               string    `gorm:"column:name;not null"`
	SolutionBag1       string    `gorm:"column:solution_bag_1;not null"`
	SolutionBag2       string    `gorm:"column:solution_bag_2;not null"`
	TotalVolumeML      int       `gorm:"column:total_volume_ml;not null"`
	TherapyTimeMinutes int       `gorm:"column:therapy_time_minutes;not null"`
	FillVolumeML       int       `gorm:"column:fill_volume_ml;not null"`
	Cycles             int       `gorm:"column:cycles;not null"`
	DwellTimeMinutes   int       `gorm:"column:dwell_time_minutes;not null"`
	LastFillML         *int      `gorm:"column:last_fill_ml"`
	ManualExchange     *string   `gorm:"column:manual_exchange"`
	IsDefaultProfile   bool      `gorm:"column:is_default_profile;not null;default:0"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (ApdPrescription) TableName() string {
	return "apd_prescriptions"
}
