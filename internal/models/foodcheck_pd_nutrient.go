package models

import "time"

// FoodCheckPDNutrient lists the 6 nutrients PD (peritoneal dialysis)
// patients need surfaced (energy, protein, phosphorus, potassium, sodium,
// moisture) and their Thai display labels. NutrientName is the canonical
// spelling (matching INMU's naming); Anamai's different spellings resolve
// to it via FoodCheckNutrientNameMap.
//
// Deliberately does NOT carry the source system's `risk_direction` column:
// it was seeded in that DB but never read by any of that system's code
// (see docs/foodcheck_survey.md 4.1). The actual traffic-light thresholds
// for the new risk indicator live in internal/foodrisk instead, so this
// table doesn't repeat that dead-config mistake.
type FoodCheckPDNutrient struct {
	ID            uint64    `gorm:"column:id;primaryKey"`
	NutrientName  string    `gorm:"column:nutrient_name;unique;not null"`
	DisplayNameTh string    `gorm:"column:display_name_th;not null"`
	Unit          string    `gorm:"column:unit;not null"`
	SortOrder     int       `gorm:"column:sort_order;not null;default:0"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (FoodCheckPDNutrient) TableName() string {
	return "foodcheck_pd_nutrients"
}
