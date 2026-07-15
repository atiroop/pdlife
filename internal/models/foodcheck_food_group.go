package models

import "time"

// FoodCheckFoodGroup is one of INMU's 17 food groups (A-Z, skipping
// I/L/O/P/R — the source database simply never assigned those letters).
type FoodCheckFoodGroup struct {
	Status      string    `gorm:"column:status;primaryKey"`
	FoodGroupID int       `gorm:"column:food_group_id;not null"`
	NameEn      string    `gorm:"column:name_en;not null"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (FoodCheckFoodGroup) TableName() string {
	return "foodcheck_food_groups"
}
