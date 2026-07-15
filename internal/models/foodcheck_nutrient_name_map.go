package models

import "time"

// FoodCheckNutrientNameMap resolves a source-specific nutrient name (e.g.
// Anamai's "Water") to the canonical name used in FoodCheckPDNutrient (e.g.
// "Moisture"), so both sources can be highlighted for PD patients the same
// way. One canonical name can have multiple source spellings (Anamai splits
// "Energy" vs "Total Energy" depending on food type).
type FoodCheckNutrientNameMap struct {
	Source        FoodCheckSource `gorm:"column:source;primaryKey;type:enum('thaifcd_inmu','thaifcd_anamai')"`
	SourceName    string          `gorm:"column:source_name;primaryKey"`
	CanonicalName string          `gorm:"column:canonical_name;not null"`
	CreatedAt     time.Time       `gorm:"column:created_at"`
	UpdatedAt     time.Time       `gorm:"column:updated_at"`
}

func (FoodCheckNutrientNameMap) TableName() string {
	return "foodcheck_nutrient_name_maps"
}
