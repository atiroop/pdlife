package models

// FoodCheckSource identifies which upstream nutrition database a Food Check
// row came from. The two sources use incompatible id spaces (INTEGER for
// INMU, zero-padded TEXT for Anamai) and different nutrient name spellings
// — see foodcheck_nutrient_name_maps and docs/foodcheck_survey.md 4.2.
type FoodCheckSource string

const (
	FoodCheckSourceINMU   FoodCheckSource = "thaifcd_inmu"
	FoodCheckSourceAnamai FoodCheckSource = "thaifcd_anamai"
)
