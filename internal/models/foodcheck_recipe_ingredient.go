package models

import "time"

// FoodCheckRecipeIngredient is one ingredient line in a FoodCheckRecipe.
// INMU-only (FoodID references foodcheck_foods), same limitation as the
// source system — see docs/foodcheck_survey.md 4.2. Placeholder only, no
// endpoints yet.
type FoodCheckRecipeIngredient struct {
	ID        uint64    `gorm:"column:id;primaryKey"`
	RecipeID  uint64    `gorm:"column:recipe_id;not null"`
	FoodID    uint64    `gorm:"column:food_id;not null"`
	AmountG   float64   `gorm:"column:amount_g;not null"`
	Note      *string   `gorm:"column:note"`
	SortOrder int       `gorm:"column:sort_order;not null;default:0"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (FoodCheckRecipeIngredient) TableName() string {
	return "foodcheck_recipe_ingredients"
}
