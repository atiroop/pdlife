package models

// Read-only models backing the SQL views created in
// migrations/20260709_create_foodcheck.sql. Never Create/Update/Delete
// against these — they exist purely for GORM to Find/Scan query results.

// FoodCheckFoodNutrientView is every nutrient value for an INMU food, with
// IsMissing flagging values that need a "no data" display instead of a
// number (source had '-' or deriv_by = "Not analysed").
type FoodCheckFoodNutrientView struct {
	FoodID       uint64   `gorm:"column:food_id"`
	FoodCode     *string  `gorm:"column:food_code"`
	NameTh       *string  `gorm:"column:name_th"`
	NameEn       *string  `gorm:"column:name_en"`
	FoodGroup    string   `gorm:"column:food_group"`
	NutrientName string   `gorm:"column:nutrient_name"`
	Unit         *string  `gorm:"column:unit"`
	PerHundredG  *float64 `gorm:"column:per_100g"`
	DerivBy      *string  `gorm:"column:deriv_by"`
	IsMissing    bool     `gorm:"column:is_missing"`
}

func (FoodCheckFoodNutrientView) TableName() string {
	return "v_foodcheck_food_nutrients"
}

// FoodCheckPDNutrientView is one of the 6 PD-critical nutrient values for
// one food, unified across INMU and Anamai. FoodUID is "thaifcd_inmu:<id>"
// or "thaifcd_anamai:<fid>" — the two sources' id spaces are incompatible,
// so this string is the only stable join key across both.
type FoodCheckPDNutrientView struct {
	FoodUID       string          `gorm:"column:food_uid"`
	Source        FoodCheckSource `gorm:"column:source"`
	FoodCode      *string         `gorm:"column:food_code"`
	NameTh        *string         `gorm:"column:name_th"`
	NameEn        *string         `gorm:"column:name_en"`
	NutrientName  string          `gorm:"column:nutrient_name"`
	DisplayNameTh string          `gorm:"column:display_name_th"`
	Unit          string          `gorm:"column:unit"`
	SortOrder     int             `gorm:"column:sort_order"`
	ValuePer100G  *float64        `gorm:"column:value_per_100g"`
	ValueSource   *string         `gorm:"column:value_source"`
}

func (FoodCheckPDNutrientView) TableName() string {
	return "v_foodcheck_pd_nutrients"
}

// FoodCheckRecipeNutritionView is the summed nutrition of one recipe for
// one PD-critical nutrient (TotalValue = sum across all ingredients,
// PerServingValue = TotalValue / Servings).
type FoodCheckRecipeNutritionView struct {
	RecipeID         uint64   `gorm:"column:recipe_id"`
	RecipeName       string   `gorm:"column:recipe_name"`
	PatientProfileID uint64   `gorm:"column:patient_profile_id"`
	Servings         float64  `gorm:"column:servings"`
	NutrientName     string   `gorm:"column:nutrient_name"`
	DisplayNameTh    string   `gorm:"column:display_name_th"`
	Unit             string   `gorm:"column:unit"`
	SortOrder        int      `gorm:"column:sort_order"`
	TotalValue       *float64 `gorm:"column:total_value"`
	PerServingValue  *float64 `gorm:"column:per_serving_value"`
}

func (FoodCheckRecipeNutritionView) TableName() string {
	return "v_foodcheck_recipe_nutrition"
}
