// Command migrate_foodcheck is a one-off, run-once-by-hand tool. It is NOT
// wired to any HTTP route and is not part of the deployed web binary (see
// cmd/migrate_apd for the identical pattern used for the APD Log Book).
//
// It copies the static reference data from foodcheck.jocky.website's
// SQLite database (INMU food groups/foods/nutrients + Anamai
// foods/nutrients) into this app's MySQL tables created by
// migrations/20260709_create_foodcheck.sql. The source database is opened
// read-only and this tool issues zero writes against it — see
// docs/foodcheck_survey.md for the full survey this port is based on.
//
// pd_nutrients and nutrient_name_maps are NOT migrated here: they're small
// fixed config tables already seeded directly by the migration SQL file
// (see its INSERT ... ON DUPLICATE KEY UPDATE statements), so there is
// nothing left for this tool to copy.
//
// Run once, from the production server (source and dest are both local
// there), after migrations/20260709_create_foodcheck.sql has been applied:
//
//	SQLITE_PATH=/home/jocky/apps/foodcheck/data/foodcheck.sqlite \
//	go run ./cmd/migrate_foodcheck
//
// Refuses to run if foodcheck_food_groups already has rows (this migration
// has already run). After copying, it re-counts every table on both sides
// and sums a few high-stakes nutrient values (potassium, phosphorus,
// sodium) on both sides as a sanity check — a mismatch there is exactly
// the kind of silent data corruption that would be dangerous for a PD
// patient reading these numbers (see docs/foodcheck_survey.md risk #1).
package main

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/atiroop/pdlife/internal/config"
	"github.com/atiroop/pdlife/internal/models"
)

func main() {
	sqlitePath := getEnvOr("SQLITE_PATH", "/home/jocky/apps/foodcheck/data/foodcheck.sqlite")

	// mode=ro is a defensive read-only guard, not the only thing keeping
	// this tool from touching the source: every source query below is a
	// plain SELECT, no writes are ever issued against this connection.
	source, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", sqlitePath))
	if err != nil {
		log.Fatalf("opening source sqlite %q failed: %v", sqlitePath, err)
	}
	defer source.Close()
	if err := source.Ping(); err != nil {
		log.Fatalf("source sqlite ping failed: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("migrate_foodcheck: %v", err)
	}
	dest, err := config.NewDB(cfg)
	if err != nil {
		log.Fatalf("destination (pdlife) DB connection failed: %v", err)
	}
	destSQL, err := dest.DB()
	if err != nil {
		log.Fatalf("destination DB handle failed: %v", err)
	}

	var existing int64
	if err := dest.Model(&models.FoodCheckFoodGroup{}).Count(&existing).Error; err != nil {
		log.Fatalf("checking for existing data failed: %v", err)
	}
	if existing > 0 {
		log.Fatalf("refusing to run: foodcheck_food_groups already has %d row(s) — this migration has already run", existing)
	}

	now := time.Now()

	migrateFoodGroups(source, destSQL, now)
	migrateFoods(source, destSQL, now)
	migrateFoodNutrients(source, destSQL, now)
	migrateAnamaiFoods(source, destSQL, now)
	migrateAnamaiNutrients(source, destSQL, now)

	log.Println("=== verification ===")
	verifyCount(source, destSQL, "food_group", "foodcheck_food_groups")
	verifyCount(source, destSQL, "food", "foodcheck_foods")
	verifyCount(source, destSQL, "nutrient", "foodcheck_food_nutrients")
	verifyCount(source, destSQL, "anamai_food", "foodcheck_anamai_foods")
	verifyCount(source, destSQL, "anamai_nutrient", "foodcheck_anamai_nutrients")

	verifyInmuNutrientSum(source, destSQL, "Potassium")
	verifyInmuNutrientSum(source, destSQL, "Phosphorus")
	verifyInmuNutrientSum(source, destSQL, "Sodium")
	verifyAnamaiNutrientSum(source, destSQL, "Potassium")
	verifyAnamaiNutrientSum(source, destSQL, "Phosphorus")
	verifyAnamaiNutrientSum(source, destSQL, "Sodium")

	log.Println("DONE")
}

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// nullFloat parses a SQLite TEXT nutrient value ('-', blank, or a number)
// into *float64. Anything that isn't cleanly parseable becomes nil (never
// 0 — a missing measurement and a measured zero are not the same thing for
// a nutrient a PD patient is watching). Malformed non-empty values are
// logged so they can be spot-checked by hand rather than silently dropped.
func nullFloat(s sql.NullString, context string) *float64 {
	if !s.Valid {
		return nil
	}
	text := strings.TrimSpace(s.String)
	if text == "" || text == "-" || text == "—" {
		return nil
	}
	text = strings.ReplaceAll(text, ",", "")
	var f float64
	if _, err := fmt.Sscanf(text, "%g", &f); err != nil || math.IsNaN(f) {
		log.Printf("WARN: %s: could not parse %q as a number — leaving NULL", context, s.String)
		return nil
	}
	return &f
}

func nullString(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

// ---- food_group -> foodcheck_food_groups ----

func migrateFoodGroups(source *sql.DB, dest *sql.DB, now time.Time) {
	rows, err := source.Query(`SELECT status, food_group_id, name_en FROM food_group ORDER BY status`)
	if err != nil {
		log.Fatalf("reading source food_group failed: %v", err)
	}
	defer rows.Close()

	stmt, err := dest.Prepare(`INSERT INTO foodcheck_food_groups (status, food_group_id, name_en, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		log.Fatalf("preparing foodcheck_food_groups insert failed: %v", err)
	}
	defer stmt.Close()

	var count int
	for rows.Next() {
		var status, nameEn string
		var foodGroupID int
		if err := rows.Scan(&status, &foodGroupID, &nameEn); err != nil {
			log.Fatalf("scanning source food_group row failed: %v", err)
		}
		if _, err := stmt.Exec(status, foodGroupID, nameEn, now, now); err != nil {
			log.Fatalf("inserting foodcheck_food_groups row (status=%s) failed: %v", status, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("iterating source food_group failed: %v", err)
	}
	log.Printf("food_group -> foodcheck_food_groups: migrated %d row(s)", count)
}

// ---- food -> foodcheck_foods ----

func migrateFoods(source *sql.DB, dest *sql.DB, now time.Time) {
	rows, err := source.Query(`
		SELECT id, food_code, status, food_group_id, name_th, name_en, scientific_name, dbcode
		FROM food ORDER BY id`)
	if err != nil {
		log.Fatalf("reading source food failed: %v", err)
	}
	defer rows.Close()

	stmt, err := dest.Prepare(`
		INSERT INTO foodcheck_foods
			(id, food_code, status, food_group_id, name_th, name_en, scientific_name, dbcode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		log.Fatalf("preparing foodcheck_foods insert failed: %v", err)
	}
	defer stmt.Close()

	var count int
	for rows.Next() {
		var id int64
		var foodGroupID int
		var status, dbcode string
		var foodCode, nameTh, nameEn, scientificName sql.NullString
		if err := rows.Scan(&id, &foodCode, &status, &foodGroupID, &nameTh, &nameEn, &scientificName, &dbcode); err != nil {
			log.Fatalf("scanning source food row failed: %v", err)
		}
		if _, err := stmt.Exec(id, nullString(foodCode), status, foodGroupID, nullString(nameTh), nullString(nameEn), nullString(scientificName), dbcode, now, now); err != nil {
			log.Fatalf("inserting foodcheck_foods row (id=%d) failed: %v", id, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("iterating source food failed: %v", err)
	}
	log.Printf("food -> foodcheck_foods: migrated %d row(s)", count)
}

// ---- nutrient -> foodcheck_food_nutrients ----

func migrateFoodNutrients(source *sql.DB, dest *sql.DB, now time.Time) {
	rows, err := source.Query(`
		SELECT food_id, nutrient_name, unit, per_100g, deriv_by, n, min_val, max_val, sd, footnote, last_updated
		FROM nutrient ORDER BY food_id`)
	if err != nil {
		log.Fatalf("reading source nutrient failed: %v", err)
	}
	defer rows.Close()

	stmt, err := dest.Prepare(`
		INSERT INTO foodcheck_food_nutrients
			(food_id, nutrient_name, unit, per_100g, deriv_by, n, min_val, max_val, sd, footnote, last_updated, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		log.Fatalf("preparing foodcheck_food_nutrients insert failed: %v", err)
	}
	defer stmt.Close()

	var count int
	for rows.Next() {
		var foodID int64
		var nutrientName string
		var unit, per100g, derivBy, n, minVal, maxVal, sd, footnote, lastUpdated sql.NullString
		if err := rows.Scan(&foodID, &nutrientName, &unit, &per100g, &derivBy, &n, &minVal, &maxVal, &sd, &footnote, &lastUpdated); err != nil {
			log.Fatalf("scanning source nutrient row failed: %v", err)
		}
		ctx := fmt.Sprintf("nutrient(food_id=%d, %s)", foodID, nutrientName)
		if _, err := stmt.Exec(
			foodID, nutrientName, nullString(unit),
			nullFloat(per100g, ctx+".per_100g"), nullString(derivBy), nullString(n),
			nullFloat(minVal, ctx+".min_val"), nullFloat(maxVal, ctx+".max_val"), nullFloat(sd, ctx+".sd"),
			nullString(footnote), nullString(lastUpdated), now, now,
		); err != nil {
			log.Fatalf("inserting %s failed: %v", ctx, err)
		}
		count++
		if count%10000 == 0 {
			log.Printf("  ... %d nutrient rows so far", count)
		}
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("iterating source nutrient failed: %v", err)
	}
	log.Printf("nutrient -> foodcheck_food_nutrients: migrated %d row(s)", count)
}

// ---- anamai_food -> foodcheck_anamai_foods ----

func migrateAnamaiFoods(source *sql.DB, dest *sql.DB, now time.Time) {
	rows, err := source.Query(`
		SELECT fid, name_th, name_en, food_group_th, food_group_en, food_type
		FROM anamai_food ORDER BY fid`)
	if err != nil {
		log.Fatalf("reading source anamai_food failed: %v", err)
	}
	defer rows.Close()

	stmt, err := dest.Prepare(`
		INSERT INTO foodcheck_anamai_foods
			(fid, name_th, name_en, food_group_th, food_group_en, food_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		log.Fatalf("preparing foodcheck_anamai_foods insert failed: %v", err)
	}
	defer stmt.Close()

	var count int
	for rows.Next() {
		var fid string
		var nameTh, nameEn, groupTh, groupEn, foodType sql.NullString
		if err := rows.Scan(&fid, &nameTh, &nameEn, &groupTh, &groupEn, &foodType); err != nil {
			log.Fatalf("scanning source anamai_food row failed: %v", err)
		}
		if _, err := stmt.Exec(fid, nullString(nameTh), nullString(nameEn), nullString(groupTh), nullString(groupEn), nullString(foodType), now, now); err != nil {
			log.Fatalf("inserting foodcheck_anamai_foods row (fid=%s) failed: %v", fid, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("iterating source anamai_food failed: %v", err)
	}
	log.Printf("anamai_food -> foodcheck_anamai_foods: migrated %d row(s)", count)
}

// ---- anamai_nutrient -> foodcheck_anamai_nutrients ----

func migrateAnamaiNutrients(source *sql.DB, dest *sql.DB, now time.Time) {
	rows, err := source.Query(`
		SELECT fid, category, nutrient_name, amount, unit
		FROM anamai_nutrient ORDER BY fid`)
	if err != nil {
		log.Fatalf("reading source anamai_nutrient failed: %v", err)
	}
	defer rows.Close()

	stmt, err := dest.Prepare(`
		INSERT INTO foodcheck_anamai_nutrients
			(fid, category, nutrient_name, amount, unit, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		log.Fatalf("preparing foodcheck_anamai_nutrients insert failed: %v", err)
	}
	defer stmt.Close()

	var count int
	for rows.Next() {
		var fid, nutrientName string
		var category, unit sql.NullString
		var amount sql.NullFloat64
		if err := rows.Scan(&fid, &category, &nutrientName, &amount, &unit); err != nil {
			log.Fatalf("scanning source anamai_nutrient row failed: %v", err)
		}
		var amountPtr *float64
		if amount.Valid {
			v := amount.Float64
			amountPtr = &v
		}
		if _, err := stmt.Exec(fid, nullString(category), nutrientName, amountPtr, nullString(unit), now, now); err != nil {
			log.Fatalf("inserting foodcheck_anamai_nutrients row (fid=%s, %s) failed: %v", fid, nutrientName, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("iterating source anamai_nutrient failed: %v", err)
	}
	log.Printf("anamai_nutrient -> foodcheck_anamai_nutrients: migrated %d row(s)", count)
}

// ---- verification ----

func verifyCount(source *sql.DB, dest *sql.DB, sourceTable, destTable string) {
	var sourceCount, destCount int64
	if err := source.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", sourceTable)).Scan(&sourceCount); err != nil {
		log.Fatalf("verify: counting source.%s failed: %v", sourceTable, err)
	}
	if err := dest.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", destTable)).Scan(&destCount); err != nil {
		log.Fatalf("verify: counting dest.%s failed: %v", destTable, err)
	}
	status := "OK"
	if sourceCount != destCount {
		status = "MISMATCH"
	}
	log.Printf("[%s] %s: source=%d dest=%s(%d)", status, sourceTable, sourceCount, destTable, destCount)
}

// verifyInmuNutrientSum sums an INMU nutrient across all foods on both
// sides. Source values are TEXT and must skip non-numeric rows exactly the
// way nullFloat does on import, or the two sums would never agree.
func verifyInmuNutrientSum(source *sql.DB, dest *sql.DB, nutrientName string) {
	rows, err := source.Query(`SELECT per_100g FROM nutrient WHERE nutrient_name = ?`, nutrientName)
	if err != nil {
		log.Fatalf("verify: reading source nutrient %s failed: %v", nutrientName, err)
	}
	var sourceSum float64
	var sourceN int
	for rows.Next() {
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			log.Fatalf("verify: scanning source nutrient %s failed: %v", nutrientName, err)
		}
		if f := nullFloat(v, "verify."+nutrientName); f != nil {
			sourceSum += *f
			sourceN++
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Fatalf("verify: iterating source nutrient %s failed: %v", nutrientName, err)
	}

	var destSum sql.NullFloat64
	var destN int64
	if err := dest.QueryRow(
		`SELECT SUM(per_100g), COUNT(*) FROM foodcheck_food_nutrients WHERE nutrient_name = ? AND per_100g IS NOT NULL`,
		nutrientName,
	).Scan(&destSum, &destN); err != nil {
		log.Fatalf("verify: reading dest foodcheck_food_nutrients %s failed: %v", nutrientName, err)
	}

	status := "OK"
	if sourceN != int(destN) || math.Abs(sourceSum-destSum.Float64) > 0.01 {
		status = "MISMATCH"
	}
	log.Printf("[%s] INMU %s sum: source=%.4f (n=%d) dest=%.4f (n=%d)", status, nutrientName, sourceSum, sourceN, destSum.Float64, destN)
}

func verifyAnamaiNutrientSum(source *sql.DB, dest *sql.DB, canonicalName string) {
	// Anamai spells this nutrient differently depending on food; resolve
	// every source_name that maps to canonicalName, same as
	// v_foodcheck_pd_nutrients does, so the comparison is apples-to-apples.
	sourceNames, err := anamaiSourceNamesFor(canonicalName)
	if err != nil {
		log.Fatalf("verify: %v", err)
	}
	placeholders := make([]string, len(sourceNames))
	args := make([]interface{}, len(sourceNames))
	for i, n := range sourceNames {
		placeholders[i] = "?"
		args[i] = n
	}
	query := fmt.Sprintf(`SELECT amount FROM anamai_nutrient WHERE nutrient_name IN (%s)`, strings.Join(placeholders, ","))
	rows, err := source.Query(query, args...)
	if err != nil {
		log.Fatalf("verify: reading source anamai_nutrient %s failed: %v", canonicalName, err)
	}
	var sourceSum float64
	var sourceN int
	for rows.Next() {
		var v sql.NullFloat64
		if err := rows.Scan(&v); err != nil {
			log.Fatalf("verify: scanning source anamai_nutrient %s failed: %v", canonicalName, err)
		}
		if v.Valid {
			sourceSum += v.Float64
			sourceN++
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Fatalf("verify: iterating source anamai_nutrient %s failed: %v", canonicalName, err)
	}

	destQuery := fmt.Sprintf(`SELECT SUM(amount), COUNT(*) FROM foodcheck_anamai_nutrients WHERE nutrient_name IN (%s) AND amount IS NOT NULL`, strings.Join(placeholders, ","))
	var destSum sql.NullFloat64
	var destN int64
	if err := dest.QueryRow(destQuery, args...).Scan(&destSum, &destN); err != nil {
		log.Fatalf("verify: reading dest foodcheck_anamai_nutrients %s failed: %v", canonicalName, err)
	}

	status := "OK"
	if sourceN != int(destN) || math.Abs(sourceSum-destSum.Float64) > 0.01 {
		status = "MISMATCH"
	}
	log.Printf("[%s] Anamai %s sum (source names: %v): source=%.4f (n=%d) dest=%.4f (n=%d)", status, canonicalName, sourceNames, sourceSum, sourceN, destSum.Float64, destN)
}

// anamaiSourceNamesFor mirrors the fixed mapping seeded into
// foodcheck_nutrient_name_maps by migrations/20260709_create_foodcheck.sql
// — kept as a literal table here rather than querying the dest DB so this
// verification step still works even before that seed data is trusted.
func anamaiSourceNamesFor(canonicalName string) ([]string, error) {
	switch canonicalName {
	case "Potassium":
		return []string{"Potassium"}, nil
	case "Phosphorus":
		return []string{"Phosphorus"}, nil
	case "Sodium":
		return []string{"Sodium"}, nil
	case "Protein, total":
		return []string{"Protein"}, nil
	case "Moisture":
		return []string{"Water"}, nil
	case "Energy, by calculation":
		return []string{"Energy", "Total Energy"}, nil
	default:
		return nil, fmt.Errorf("no known Anamai source_name mapping for canonical nutrient %q", canonicalName)
	}
}
