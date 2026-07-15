package models

import "time"

// FoodCheckRecipe is a patient's saved recipe (Recipe Builder). Placeholder
// only — migrated as a table shape because the source schema had it, but
// there are no endpoints yet (the source never shipped a UI for this
// either; see docs/foodcheck_survey.md 7). Tied to PatientProfileID rather
// than a separate foodcheck user table.
type FoodCheckRecipe struct {
	ID               uint64    `gorm:"column:id;primaryKey"`
	PatientProfileID uint64    `gorm:"column:patient_profile_id;not null"`
	Name             string    `gorm:"column:name;not null"`
	Description      *string   `gorm:"column:description"`
	Servings         float64   `gorm:"column:servings;not null;default:1"`
	ServingUnit      string    `gorm:"column:serving_unit;not null;default:จาน"`
	IsPublic         bool      `gorm:"column:is_public;not null;default:0"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (FoodCheckRecipe) TableName() string {
	return "foodcheck_recipes"
}
