package models

import "time"

// FoodCheckFoodNutrient is one nutrient value for one INMU food (long
// format: one row per nutrient per food). PerHundredG is NULL where the
// source had no data (the literal string '-') or free-text noise instead
// of a number — never coerced to 0, since a missing value and a
// measured-zero value mean very different things for a PD patient watching
// potassium/phosphorus. See docs/foodcheck_survey.md risk #1.
type FoodCheckFoodNutrient struct {
	FoodID       uint64    `gorm:"column:food_id;primaryKey"`
	NutrientName string    `gorm:"column:nutrient_name;primaryKey"`
	Unit         *string   `gorm:"column:unit"`
	PerHundredG  *float64  `gorm:"column:per_100g"`
	DerivBy      *string   `gorm:"column:deriv_by"`
	N            *string   `gorm:"column:n"`
	MinVal       *float64  `gorm:"column:min_val"`
	MaxVal       *float64  `gorm:"column:max_val"`
	SD           *float64  `gorm:"column:sd"`
	Footnote     *string   `gorm:"column:footnote"`
	LastUpdated  *string   `gorm:"column:last_updated"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (FoodCheckFoodNutrient) TableName() string {
	return "foodcheck_food_nutrients"
}
