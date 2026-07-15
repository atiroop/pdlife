package models

import "time"

// FoodCheckFood is one INMU (Thai FCD) food item. ID is preserved verbatim
// from the source's internal id (not auto-increment) so migrated
// FoodCheckFoodNutrient rows FK onto it unchanged.
type FoodCheckFood struct {
	ID             uint64    `gorm:"column:id;primaryKey"`
	FoodCode       *string   `gorm:"column:food_code"`
	Status         string    `gorm:"column:status;not null"`
	FoodGroupID    int       `gorm:"column:food_group_id;not null"`
	NameTh         *string   `gorm:"column:name_th"`
	NameEn         *string   `gorm:"column:name_en"`
	ScientificName *string   `gorm:"column:scientific_name"`
	DBCode         string    `gorm:"column:dbcode;not null;default:STD"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (FoodCheckFood) TableName() string {
	return "foodcheck_foods"
}
