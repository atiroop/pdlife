package models

import "time"

type TreatmentType string

const (
	TreatmentCAPD TreatmentType = "CAPD"
	TreatmentAPD  TreatmentType = "APD"
	TreatmentHD   TreatmentType = "HD"
)

type CoverageType string

const (
	CoverageGoldCard     CoverageType = "บัตรทอง"
	CoverageSocialSecure CoverageType = "ประกันสังคม"
	CoverageCivilServant CoverageType = "ข้าราชการ"
	CoverageOther        CoverageType = "อื่นๆ"
)

type PatientProfile struct {
	ID                 uint64         `gorm:"column:id;primaryKey"`
	UserID             uint64         `gorm:"column:user_id;unique;not null"`
	TreatmentType      *TreatmentType `gorm:"column:treatment_type;type:enum('CAPD','APD','HD')"`
	HospitalName       *string        `gorm:"column:hospital_name"`
	CoverageType       *CoverageType  `gorm:"column:coverage_type;type:enum('บัตรทอง','ประกันสังคม','ข้าราชการ','อื่นๆ')"`
	ProfileCompletedAt *time.Time     `gorm:"column:profile_completed_at"`
	CreatedAt          time.Time      `gorm:"column:created_at"`
	UpdatedAt          time.Time      `gorm:"column:updated_at"`
}

func (PatientProfile) TableName() string {
	return "patient_profiles"
}
