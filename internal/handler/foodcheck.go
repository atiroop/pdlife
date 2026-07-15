package handler

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/foodrisk"
	"github.com/atiroop/pdlife/internal/foodunit"
	"github.com/atiroop/pdlife/internal/models"
)

// pdNutrientOrder is the display order for the 6 PD-critical nutrients,
// matching foodcheck_pd_nutrients.sort_order (energy, protein, phosphorus,
// potassium, sodium, moisture). Kept as a literal here rather than an
// extra query per request — this list changes only if the DB seed changes.
var pdNutrientOrder = []string{
	"Energy, by calculation",
	"Protein, total",
	"Phosphorus",
	"Potassium",
	"Sodium",
	"Moisture",
}

// requireLoggedInMember gates every /food-check route: a valid session, a
// verified email, and health-data consent. Food Check isn't tied to a
// treatment type the way /apd or /capd are, but its search history is
// listed as sensitive personal data in the privacy policy (§2.2), so it
// requires the same consent gate — which in turn requires a completed
// PatientProfile row to hold that consent (consent lives on
// patient_profiles, see internal/models/patient_profile.go), so this now
// also redirects to /onboarding first if the profile isn't complete yet.
func (h *AuthHandler) requireLoggedInMember(c echo.Context) (*models.User, error) {
	user, err := h.currentSession(c)
	if err != nil {
		return nil, c.Redirect(http.StatusSeeOther, "/login")
	}
	if user.Role == models.RoleUnverified {
		return nil, c.Render(http.StatusForbidden, "placeholder.html", map[string]string{
			"Title":   "ยืนยันอีเมลก่อน",
			"Message": "กรุณายืนยันอีเมลก่อนใช้งาน Food Check",
		})
	}
	var profile models.PatientProfile
	if err := h.DB.Where("user_id = ?", user.ID).First(&profile).Error; err != nil || profile.ProfileCompletedAt == nil {
		return nil, c.Redirect(http.StatusSeeOther, "/onboarding")
	}
	if profile.HealthDataConsentAt == nil {
		return nil, c.Redirect(http.StatusSeeOther, "/consent")
	}
	return user, nil
}

// foodCheckBadge pairs a nutrient's Thai label/value with its risk badge,
// in foodcheck_pd_nutrients display order, for a template to range over.
type foodCheckBadge struct {
	NutrientName  string
	DisplayNameTh string
	Unit          string
	ValuePer100G  *float64
	Badge         foodrisk.Badge
}

// foodCheckResultRow is one row of the compact search-results table.
// Named fields (rather than the generic []foodCheckBadge slice
// FoodCheckDetail uses) so the table template can address each nutrient
// column directly without an index-position assumption.
type foodCheckResultRow struct {
	Source     models.FoodCheckSource
	Ref        string // foodcheck_foods.id or foodcheck_anamai_foods.fid, as text
	FoodCode   string
	NameTh     string
	NameEn     string
	Energy     foodCheckBadge
	Protein    foodCheckBadge
	Phosphorus foodCheckBadge
	Potassium  foodCheckBadge
	Sodium     foodCheckBadge
}

// ---- GET /food-check ----

func (h *AuthHandler) FoodCheckSearch(c echo.Context) error {
	user, err := h.requireLoggedInMember(c)
	if user == nil {
		return err
	}

	q := strings.TrimSpace(c.QueryParam("q"))
	group := strings.ToUpper(strings.TrimSpace(c.QueryParam("group")))

	var groups []models.FoodCheckFoodGroup
	h.DB.Order("status").Find(&groups)

	var results []foodCheckResultRow
	if len([]rune(q)) >= 2 || group != "" {
		results = h.foodCheckSearch(q, group)
	}

	data := map[string]interface{}{
		"Query":      q,
		"Group":      group,
		"Groups":     groups,
		"Results":    results,
		"Searched":   len([]rune(q)) >= 2 || group != "",
		"Disclaimer": foodrisk.Disclaimer,
	}
	return c.Render(http.StatusOK, "foodcheck_search.html", withNav(data, user, h.navInfoForUser(user), "/food-check"))
}

// foodCheckSearch runs the INMU and Anamai searches, evaluates badges for
// every result's 6 PD nutrients, and returns a merged, name-sorted,
// 40-row-capped list — same shape as the source system's /api/search (see
// docs/foodcheck_survey.md 3.1), including the Anamai limitation that a
// group filter only applies to INMU (Anamai has no A-Z taxonomy).
func (h *AuthHandler) foodCheckSearch(q, group string) []foodCheckResultRow {
	var rows []foodCheckResultRow
	var foodUIDs []string
	uidToRowIndex := map[string]int{}

	// INMU search always runs — it's the only source with an A-Z group
	// filter, so `group` (if set) applies here.
	{
		var foods []models.FoodCheckFood
		query := h.DB.Model(&models.FoodCheckFood{})
		if q != "" {
			like := "%" + q + "%"
			query = query.Where("name_th LIKE ? OR name_en LIKE ? OR food_code LIKE ?", like, like, like)
		}
		if group != "" {
			query = query.Where("status = ?", group)
		}
		query.Order("name_th").Limit(30).Find(&foods)

		for _, f := range foods {
			ref := strconv.FormatUint(f.ID, 10)
			uid := string(models.FoodCheckSourceINMU) + ":" + ref
			row := foodCheckResultRow{
				Source:   models.FoodCheckSourceINMU,
				Ref:      ref,
				NameTh:   derefStr(f.NameTh),
				NameEn:   derefStr(f.NameEn),
				FoodCode: derefStr(f.FoodCode),
			}
			uidToRowIndex[uid] = len(rows)
			foodUIDs = append(foodUIDs, uid)
			rows = append(rows, row)
		}
	}

	// Anamai has no A-Z group taxonomy — skip it when filtering by group,
	// same restriction the source system had (docs/foodcheck_survey.md 3.1).
	if group == "" && q != "" {
		var afoods []models.FoodCheckAnamaiFood
		like := "%" + q + "%"
		h.DB.Where("name_th LIKE ? OR name_en LIKE ? OR fid LIKE ?", like, like, like).
			Order("name_th").Limit(30).Find(&afoods)

		for _, af := range afoods {
			uid := string(models.FoodCheckSourceAnamai) + ":" + af.Fid
			row := foodCheckResultRow{
				Source:   models.FoodCheckSourceAnamai,
				Ref:      af.Fid,
				NameTh:   derefStr(af.NameTh),
				NameEn:   derefStr(af.NameEn),
				FoodCode: af.Fid,
			}
			uidToRowIndex[uid] = len(rows)
			foodUIDs = append(foodUIDs, uid)
			rows = append(rows, row)
		}
	}

	if len(rows) == 0 {
		return rows
	}

	var pdRows []models.FoodCheckPDNutrientView
	h.DB.Where("food_uid IN ?", foodUIDs).Find(&pdRows)

	pdByUID := map[string][]models.FoodCheckPDNutrientView{}
	for _, pr := range pdRows {
		pdByUID[pr.FoodUID] = append(pdByUID[pr.FoodUID], pr)
	}
	for uid, idx := range uidToRowIndex {
		badges := buildBadges(pdByUID[uid])
		rows[idx].Energy = badgeByName(badges, "Energy, by calculation")
		rows[idx].Protein = badgeByName(badges, "Protein, total")
		rows[idx].Phosphorus = badgeByName(badges, "Phosphorus")
		rows[idx].Potassium = badgeByName(badges, "Potassium")
		rows[idx].Sodium = badgeByName(badges, "Sodium")
	}

	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].NameTh < rows[j].NameTh
	})
	if len(rows) > 40 {
		rows = rows[:40]
	}
	return rows
}

// buildBadges evaluates every PD-critical nutrient found for one food into
// display order, skipping any canonical nutrient this food has no row for
// at all (v_foodcheck_pd_nutrients always returns all 6 via CROSS JOIN, so
// in practice this only trims if a lookup was incomplete).
func buildBadges(views []models.FoodCheckPDNutrientView) []foodCheckBadge {
	byName := map[string]models.FoodCheckPDNutrientView{}
	for _, v := range views {
		byName[v.NutrientName] = v
	}
	badges := make([]foodCheckBadge, 0, len(pdNutrientOrder))
	for _, name := range pdNutrientOrder {
		v, ok := byName[name]
		if !ok {
			continue
		}
		badges = append(badges, foodCheckBadge{
			NutrientName:  v.NutrientName,
			DisplayNameTh: v.DisplayNameTh,
			Unit:          v.Unit,
			ValuePer100G:  v.ValuePer100G,
			Badge:         foodrisk.Evaluate(v.NutrientName, v.ValuePer100G),
		})
	}
	return badges
}

// badgeByName finds one nutrient's badge by canonical name within an
// already-built badge list, or a zero-value (Level "") if absent — the
// table template treats an empty Level the same as "unknown" (renders
// "—"), which only happens if a lookup was somehow incomplete.
func badgeByName(badges []foodCheckBadge, name string) foodCheckBadge {
	for _, b := range badges {
		if b.NutrientName == name {
			return b
		}
	}
	return foodCheckBadge{}
}

// ---- GET /food-check/results (AJAX partial — same query logic as
// FoodCheckSearch, but renders only the results table fragment for the
// instant-search JS in foodcheck_search.html to swap into the page) ----

func (h *AuthHandler) FoodCheckSearchResults(c echo.Context) error {
	user, err := h.requireLoggedInMember(c)
	if user == nil {
		return err
	}

	q := strings.TrimSpace(c.QueryParam("q"))
	group := strings.ToUpper(strings.TrimSpace(c.QueryParam("group")))
	searched := len([]rune(q)) >= 2 || group != ""

	var results []foodCheckResultRow
	if searched {
		results = h.foodCheckSearch(q, group)
	}

	return c.Render(http.StatusOK, "fc-results-fragment", map[string]interface{}{
		"Results":  results,
		"Searched": searched,
	})
}

// foodCheckAllNutrientRow is one nutrient's raw source data for a food's
// detail page: NutrientName as the source spells it (displayed as-is),
// CanonicalName resolved to foodcheck_pd_nutrients spelling (for the
// PDNames highlight check and for finding "Density" — see
// findNutrientValue below).
type foodCheckAllNutrientRow struct {
	NutrientName  string
	CanonicalName string
	Unit          string
	PerHundredG   *float64
	DerivBy       string
}

// foodCheckLoaded is everything FoodCheckDetail and FoodCheckNutrition
// both need after resolving :source/:ref — loadFoodCheckFood is the single
// place that branches on source, so the two handlers can never drift on
// how a food's identity/nutrients are read.
type foodCheckLoaded struct {
	NameTh         string
	NameEn         string
	FoodCode       string
	GroupName      string
	ScientificName string
	Status         string // INMU: food_group status code; Anamai: food_type (see foodunit.FoodInfo doc comment)
	FoodUID        string
	AllNutrients   []foodCheckAllNutrientRow
}

// loadFoodCheckFood resolves a food by :source/:ref, or returns ok=false
// if the source is unrecognized or the food doesn't exist.
func (h *AuthHandler) loadFoodCheckFood(source, ref string) (loaded *foodCheckLoaded, ok bool) {
	switch source {
	case "inmu":
		id, convErr := strconv.ParseUint(ref, 10, 64)
		if convErr != nil {
			return nil, false
		}
		var food models.FoodCheckFood
		if err := h.DB.First(&food, id).Error; err != nil {
			return nil, false
		}
		var group models.FoodCheckFoodGroup
		h.DB.Where("status = ?", food.Status).First(&group)

		loaded = &foodCheckLoaded{
			NameTh:         derefStr(food.NameTh),
			NameEn:         derefStr(food.NameEn),
			FoodCode:       derefStr(food.FoodCode),
			GroupName:      group.NameEn,
			ScientificName: derefStr(food.ScientificName),
			Status:         food.Status,
			FoodUID:        string(models.FoodCheckSourceINMU) + ":" + ref,
		}

		var nutrients []models.FoodCheckFoodNutrient
		h.DB.Where("food_id = ?", id).Find(&nutrients)
		for _, n := range nutrients {
			// INMU's own spelling is already canonical (foodcheck_pd_nutrients
			// was seeded from it) — no name-map lookup needed here.
			loaded.AllNutrients = append(loaded.AllNutrients, foodCheckAllNutrientRow{
				NutrientName: n.NutrientName, CanonicalName: n.NutrientName,
				Unit: derefStr(n.Unit), PerHundredG: n.PerHundredG, DerivBy: derefStr(n.DerivBy),
			})
		}
		return loaded, true

	case "anamai":
		var food models.FoodCheckAnamaiFood
		if err := h.DB.Where("fid = ?", ref).First(&food).Error; err != nil {
			return nil, false
		}
		loaded = &foodCheckLoaded{
			NameTh:    derefStr(food.NameTh),
			NameEn:    derefStr(food.NameEn),
			FoodCode:  food.Fid,
			GroupName: derefStr(food.FoodGroupTh),
			Status:    derefStr(food.FoodType),
			FoodUID:   string(models.FoodCheckSourceAnamai) + ":" + ref,
		}

		// Anamai spells some PD-critical nutrients differently from the
		// canonical (INMU-derived) name — e.g. "Water" vs "Moisture" — so
		// the highlight/badge lookup below needs the canonical name, not
		// the raw one. Resolve via foodcheck_nutrient_name_maps rather
		// than assuming NutrientName == CanonicalName like the INMU branch
		// does (that assumption doesn't hold for Anamai).
		var nameMaps []models.FoodCheckNutrientNameMap
		h.DB.Where("source = ?", models.FoodCheckSourceAnamai).Find(&nameMaps)
		canonicalBySourceName := map[string]string{}
		for _, m := range nameMaps {
			canonicalBySourceName[m.SourceName] = m.CanonicalName
		}

		var nutrients []models.FoodCheckAnamaiNutrient
		h.DB.Where("fid = ?", ref).Find(&nutrients)
		for _, n := range nutrients {
			canonical := n.NutrientName
			if mapped, ok := canonicalBySourceName[n.NutrientName]; ok {
				canonical = mapped
			}
			loaded.AllNutrients = append(loaded.AllNutrients, foodCheckAllNutrientRow{
				NutrientName: n.NutrientName, CanonicalName: canonical,
				Unit: derefStr(n.Unit), PerHundredG: n.Amount,
			})
		}
		return loaded, true

	default:
		return nil, false
	}
}

// foodInfoFromLoaded adapts a loaded food's identity fields to
// foodunit.FoodInfo's shape for density fallback keyword matching.
func foodInfoFromLoaded(loaded *foodCheckLoaded) foodunit.FoodInfo {
	return foodunit.FoodInfo{
		NameTh:         loaded.NameTh,
		NameEn:         loaded.NameEn,
		ScientificName: loaded.ScientificName,
		GroupName:      loaded.GroupName,
		FoodCode:       loaded.FoodCode,
		Status:         loaded.Status,
	}
}

// conversionUnitView is one <option> the amount/unit picker renders.
// GpuStr is the full-precision grams-per-unit as a plain decimal string
// (empty when unavailable) — deliberately NOT run through fmtNutrient,
// which rounds to 1 decimal place for human display and would silently
// truncate e.g. 18.045 to "18", corrupting the client-side unit-switch
// math in foodcheck_detail.html's JS (caught during testing: switching
// units drifted the equivalent amount by ~0.25%).
type conversionUnitView struct {
	Code      string
	Label     string
	Available bool
	GpuStr    string
}

func conversionUnitViews(conv foodunit.Conversions) []conversionUnitView {
	views := make([]conversionUnitView, 0, len(conv.Units))
	for _, u := range conv.Units {
		v := conversionUnitView{Code: u.Code, Label: u.Label, Available: u.Available}
		if u.GramsPerUnit != nil {
			v.GpuStr = strconv.FormatFloat(*u.GramsPerUnit, 'f', -1, 64)
		}
		views = append(views, v)
	}
	return views
}

// findNutrientValue looks up one nutrient's per-100g value by its raw
// source name (not canonical) — used to find the "Density" nutrient,
// which only ever appears under that exact raw name for INMU foods and
// never appears at all for Anamai foods (see docs/foodcheck_survey.md).
func findNutrientValue(nutrients []foodCheckAllNutrientRow, name string) *float64 {
	for _, n := range nutrients {
		if n.NutrientName == name {
			return n.PerHundredG
		}
	}
	return nil
}

// ---- GET /food-check/food/:source/:ref ----

func (h *AuthHandler) FoodCheckDetail(c echo.Context) error {
	user, err := h.requireLoggedInMember(c)
	if user == nil {
		return err
	}

	source := c.Param("source")
	ref := c.Param("ref")

	loaded, ok := h.loadFoodCheckFood(source, ref)
	if !ok {
		return c.Render(http.StatusNotFound, "placeholder.html", map[string]string{
			"Title": "ไม่พบรายการอาหาร", "Message": "ไม่พบรายการอาหารที่ค้นหา",
		})
	}

	var pdRows []models.FoodCheckPDNutrientView
	h.DB.Where("food_uid = ?", loaded.FoodUID).Find(&pdRows)
	badges := buildBadges(pdRows)
	pdNames := map[string]bool{}
	for _, n := range pdNutrientOrder {
		pdNames[n] = true
	}

	conversions := foodunit.GetUnitConversions(
		foodInfoFromLoaded(loaded),
		findNutrientValue(loaded.AllNutrients, "Density"),
	)

	data := map[string]interface{}{
		"Source":       source,
		"Ref":          ref,
		"NameTh":       loaded.NameTh,
		"NameEn":       loaded.NameEn,
		"FoodCode":     loaded.FoodCode,
		"GroupName":    loaded.GroupName,
		"Badges":       badges,
		"AllNutrients": loaded.AllNutrients,
		"PDNames":      pdNames,
		"Disclaimer":   foodrisk.Disclaimer,
		"Conversions":  conversions,
		"ConvUnits":    conversionUnitViews(conversions),
	}
	return c.Render(http.StatusOK, "foodcheck_detail.html", withNav(data, user, h.navInfoForUser(user), "/food-check"))
}

// nutritionUnit/nutritionPDNutrient/nutritionNutrient/nutritionResponse are
// FoodCheckNutrition's JSON response shape — deliberately its own DTOs
// rather than marshaling foodunit.UnitConversion/foodrisk.Badge directly,
// so the wire format (snake_case keys) doesn't couple to those packages'
// internal field names.
type nutritionUnit struct {
	Code      string `json:"code"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
}

type nutritionPDNutrient struct {
	NutrientName  string   `json:"nutrient_name"`
	DisplayNameTh string   `json:"display_name_th"`
	Unit          string   `json:"unit"`
	Value         *float64 `json:"value"`
	BadgeLevel    string   `json:"badge_level"`
	BadgeCSSClass string   `json:"badge_css_class"`
	BadgeText     string   `json:"badge_text"`
	BadgeDetail   string   `json:"badge_detail,omitempty"`
	BadgeTooltip  string   `json:"badge_tooltip,omitempty"`
}

type nutritionNutrient struct {
	NutrientName string   `json:"nutrient_name"`
	Unit         string   `json:"unit"`
	Value        *float64 `json:"value"`
}

type nutritionResponse struct {
	Grams            float64               `json:"grams"`
	DensityAvailable bool                  `json:"density_available"`
	DensitySource    string                `json:"density_source,omitempty"`
	IsFallback       bool                  `json:"is_fallback"`
	DensityNote      string                `json:"density_note,omitempty"`
	AvailableUnits   []nutritionUnit       `json:"available_units"`
	PDNutrients      []nutritionPDNutrient `json:"pd_nutrients"`
	AllNutrients     []nutritionNutrient   `json:"all_nutrients"`
}

// ---- GET /food-check/food/:source/:ref/nutrition?amount=X&unit=Y ----
//
// Returns every nutrient value (the 6 PD-critical ones with their risk
// badges recomputed, plus the full nutrient table) scaled to the amount
// actually eaten, in whichever unit the patient picked. Badges are
// evaluated by feeding foodrisk.Evaluate the *scaled* value instead of
// the raw per-100g value — foodrisk's thresholds are plain numeric
// constants with no unit conversion baked in, so this effectively turns
// "mg per 100g" thresholds into "mg per portion actually eaten" without
// needing new thresholds (see internal/foodrisk/thresholds.go — a food
// safely under a threshold at 100g can cross into watch/alert at a larger
// portion, which is the whole point of this endpoint).
func (h *AuthHandler) FoodCheckNutrition(c echo.Context) error {
	user, err := h.requireLoggedInMember(c)
	if user == nil {
		return err
	}
	_ = user

	source := c.Param("source")
	ref := c.Param("ref")

	loaded, ok := h.loadFoodCheckFood(source, ref)
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "ไม่พบรายการอาหาร"})
	}

	amount := 100.0
	if amountStr := c.QueryParam("amount"); amountStr != "" {
		v, parseErr := strconv.ParseFloat(amountStr, 64)
		if parseErr != nil || v <= 0 || math.IsInf(v, 0) || math.IsNaN(v) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "จำนวนไม่ถูกต้อง"})
		}
		amount = v
	}
	unit := c.QueryParam("unit")
	if unit == "" {
		unit = "g"
	}

	conv := foodunit.GetUnitConversions(
		foodInfoFromLoaded(loaded),
		findNutrientValue(loaded.AllNutrients, "Density"),
	)
	grams := foodunit.GramsFor(amount, unit, conv)
	if grams == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "หน่วยนี้ใช้ไม่ได้กับอาหารรายการนี้"})
	}

	var pdRows []models.FoodCheckPDNutrientView
	h.DB.Where("food_uid = ?", loaded.FoodUID).Find(&pdRows)
	byName := map[string]models.FoodCheckPDNutrientView{}
	for _, v := range pdRows {
		byName[v.NutrientName] = v
	}

	pdOut := make([]nutritionPDNutrient, 0, len(pdNutrientOrder))
	for _, name := range pdNutrientOrder {
		v, found := byName[name]
		if !found {
			continue
		}
		var scaled *float64
		if v.ValuePer100G != nil {
			s := foodunit.ScaleValue(*v.ValuePer100G, *grams)
			scaled = &s
		}
		badge := foodrisk.Evaluate(v.NutrientName, scaled)
		pdOut = append(pdOut, nutritionPDNutrient{
			NutrientName:  v.NutrientName,
			DisplayNameTh: v.DisplayNameTh,
			Unit:          v.Unit,
			Value:         scaled,
			BadgeLevel:    string(badge.Level),
			BadgeCSSClass: badge.Level.CSSClass(),
			BadgeText:     badge.Text,
			BadgeDetail:   badge.Detail,
			BadgeTooltip:  badge.Tooltip,
		})
	}

	allOut := make([]nutritionNutrient, 0, len(loaded.AllNutrients))
	for _, n := range loaded.AllNutrients {
		var scaled *float64
		if n.PerHundredG != nil {
			s := foodunit.ScaleValue(*n.PerHundredG, *grams)
			scaled = &s
		}
		allOut = append(allOut, nutritionNutrient{NutrientName: n.NutrientName, Unit: n.Unit, Value: scaled})
	}

	unitsOut := make([]nutritionUnit, 0, len(conv.Units))
	for _, u := range conv.Units {
		unitsOut = append(unitsOut, nutritionUnit{Code: u.Code, Label: u.Label, Available: u.Available})
	}

	return c.JSON(http.StatusOK, nutritionResponse{
		Grams:            *grams,
		DensityAvailable: conv.DensityAvailable,
		DensitySource:    conv.DensitySource,
		IsFallback:       conv.IsFallback,
		DensityNote:      conv.Note,
		AvailableUnits:   unitsOut,
		PDNutrients:      pdOut,
		AllNutrients:     allOut,
	})
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
