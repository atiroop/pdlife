package models

import "time"

// FoodCheckSearchHistory records what a patient searched for in Food
// Check and, if they opened a result, which food. New table — the source
// system had no search log at all (see docs/foodcheck_survey.md 2), so
// there is nothing to migrate into it; it starts empty.
type FoodCheckSearchHistory struct {
	ID               uint64           `gorm:"column:id;primaryKey"`
	PatientProfileID uint64           `gorm:"column:patient_profile_id;not null"`
	Query            string           `gorm:"column:query;not null"`
	FoodSource       *FoodCheckSource `gorm:"column:food_source;type:enum('thaifcd_inmu','thaifcd_anamai')"`
	FoodRef          *string          `gorm:"column:food_ref"`
	SearchedAt       time.Time        `gorm:"column:searched_at;not null"`
}

func (FoodCheckSearchHistory) TableName() string {
	return "foodcheck_search_history"
}
