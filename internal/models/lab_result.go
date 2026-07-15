package models

import "time"

// LabResultFlag is the negative/positive result of a serology test
// (HBsAg/HBsAb/Anti HCV/Anti HIV).
type LabResultFlag string

const (
	LabResultNegative LabResultFlag = "negative"
	LabResultPositive LabResultFlag = "positive"
)

// LabResult is one lab-test visit's results — one row per log_date, like
// ApdLogEntry/HdLogEntry, but almost every column is nullable: unlike a
// daily log, a lab panel doesn't test everything on the same visit (some
// values are drawn every 3 months, others every 6-12 months), so a
// patient fills in only whatever was actually tested that day. See
// docs/schema_spec.md and internal/labrange for the reference ranges used
// to flag abnormal values.
type LabResult struct {
	ID               uint64    `gorm:"column:id;primaryKey"`
	PatientProfileID uint64    `gorm:"column:patient_profile_id;not null"`
	LogDate          time.Time `gorm:"column:log_date;type:date;not null"`

	// ตรวจทุก 3 เดือน
	Hct           *float64 `gorm:"column:hct"`
	Hb            *float64 `gorm:"column:hb"`
	WBC           *int     `gorm:"column:wbc"`
	PlateletCount *int     `gorm:"column:platelet_count"`
	BUN           *float64 `gorm:"column:bun"`
	Cr            *float64 `gorm:"column:cr"`
	Na            *float64 `gorm:"column:na"`
	K             *float64 `gorm:"column:k"`
	CO2           *float64 `gorm:"column:co2"`
	Ca            *float64 `gorm:"column:ca"`
	PO4           *float64 `gorm:"column:po4"`
	Albumin       *float64 `gorm:"column:albumin"`
	// KtVValue is the measured value only — never auto-classified as
	// normal/abnormal (see internal/labrange), since the target depends on
	// the patient's own dialysis schedule (2x vs 3x/week).
	KtVValue *float64 `gorm:"column:kt_v_value"`
	URR      *float64 `gorm:"column:urr"`  // HD only
	NPCR     *float64 `gorm:"column:npcr"` // HD only

	// ตรวจทุก 6 เดือน / 1 ปี
	FBS         *float64       `gorm:"column:fbs"`
	HbA1C       *float64       `gorm:"column:hba1c"`
	UricAcid    *float64       `gorm:"column:uric_acid"`
	PTH         *float64       `gorm:"column:pth"`
	Ferritin    *float64       `gorm:"column:ferritin"`
	SerumIron   *float64       `gorm:"column:serum_iron"`
	TIBC        *float64       `gorm:"column:tibc"`
	TSatPercent *float64       `gorm:"column:t_sat_percent"`
	Chol        *float64       `gorm:"column:chol"`
	HDL         *float64       `gorm:"column:hdl"`
	LDL         *float64       `gorm:"column:ldl"`
	HBsAg       *LabResultFlag `gorm:"column:hbsag;type:enum('negative','positive')"`
	HBsAb       *LabResultFlag `gorm:"column:hbsab;type:enum('negative','positive')"`
	AntiHCV     *LabResultFlag `gorm:"column:anti_hcv;type:enum('negative','positive')"`
	AntiHIV     *LabResultFlag `gorm:"column:anti_hiv;type:enum('negative','positive')"`
	CXRFinding  *string        `gorm:"column:cxr_finding"`
	EKGFinding  *string        `gorm:"column:ekg_finding"`

	Notes     *string   `gorm:"column:notes"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (LabResult) TableName() string {
	return "lab_results"
}
