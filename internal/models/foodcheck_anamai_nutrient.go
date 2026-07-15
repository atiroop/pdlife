package models

import "time"

// FoodCheckAnamaiNutrient is one nutrient value for one Anamai food (long
// format, same shape as FoodCheckFoodNutrient but a different taxonomy —
// Category groups nutrients as 'Main nutrients' | 'Minerals' | 'Vitamins' |
// ..., which INMU's data doesn't have).
type FoodCheckAnamaiNutrient struct {
	Fid          string    `gorm:"column:fid;primaryKey"`
	NutrientName string    `gorm:"column:nutrient_name;primaryKey"`
	Category     *string   `gorm:"column:category"`
	Amount       *float64  `gorm:"column:amount"`
	Unit         *string   `gorm:"column:unit"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (FoodCheckAnamaiNutrient) TableName() string {
	return "foodcheck_anamai_nutrients"
}
