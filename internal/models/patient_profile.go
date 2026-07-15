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
	// HealthDataConsentAt/Version track explicit consent to process
	// sensitive health data (PDPA section 26) — separate from
	// ProfileCompletedAt because it can be withdrawn independently
	// without erasing the rest of the profile. NULL means not given (or
	// withdrawn). Version records which dated revision of the privacy
	// policy the user consented under (see handler.HealthDataConsentVersion).
	HealthDataConsentAt      *time.Time `gorm:"column:health_data_consent_at"`
	HealthDataConsentVersion *string    `gorm:"column:health_data_consent_version"`
	CreatedAt                time.Time  `gorm:"column:created_at"`
	UpdatedAt                time.Time  `gorm:"column:updated_at"`
}

func (PatientProfile) TableName() string {
	return "patient_profiles"
}
