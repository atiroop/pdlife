package models

import "time"

// FoodCheckAnamaiFood is one food item from กรมอนามัย (Anamai). Fid is a
// zero-padded 5-digit id (e.g. "07034"), or starts with "R" for Branded
// Food Products — kept as text verbatim from the source, never parsed as a
// number.
type FoodCheckAnamaiFood struct {
	Fid         string    `gorm:"column:fid;primaryKey"`
	NameTh      *string   `gorm:"column:name_th"`
	NameEn      *string   `gorm:"column:name_en"`
	FoodGroupTh *string   `gorm:"column:food_group_th"`
	FoodGroupEn *string   `gorm:"column:food_group_en"`
	FoodType    *string   `gorm:"column:food_type"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (FoodCheckAnamaiFood) TableName() string {
	return "foodcheck_anamai_foods"
}
