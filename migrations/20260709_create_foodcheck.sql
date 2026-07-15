-- Migration: Food Check (ported from foodcheck.jocky.website — see docs/foodcheck_survey.md)
--
-- Source was SQLite with 12 tables + 3 views. Deliberate differences from
-- the source schema, decided during the port (see docs/foodcheck_survey.md
-- section 7 for why):
--   - usda_food_mapping / usda_nutrient_cache dropped entirely: 0 rows in
--     the source DB, no code ever called the USDA API. Not carried forward.
--   - source `user` table dropped: recipes/search history hang off this
--     app's own `patient_profiles` instead of a separate foodcheck user.
--   - pd_nutrient.risk_direction dropped: seeded in the source DB but never
--     read by any source code (the old frontend hardcoded the PD nutrient
--     list instead). The new risk-indicator thresholds live in
--     internal/foodrisk (Go config), not this table — see that package's
--     doc comment for why DB-driven "risk_direction" was a dead end before.
--   - nutrient.per_100g (and min_val/max_val/sd) stored as DECIMAL here,
--     not TEXT: the source mixed real numbers with the literal string '-'
--     for missing values. Cast to NULL during import instead of parsing on
--     every read (see foodcheck_survey.md risk #1 — historically the
--     highest-risk part of this port).
--   - `foodcheck_` prefix on every table, matching the existing `apd_`
--     prefix convention for feature-specific tables in this codebase.
--
-- food_group/food/nutrient (INMU) and anamai_food/anamai_nutrient stay as
-- separate table pairs, same as the source, because the two sources use
-- incompatible id spaces (INTEGER vs zero-padded TEXT) and different
-- nutrient taxonomies — merging them into one table was considered and
-- rejected as unnecessary churn for a one-time import.

-- =============================================================================
-- SECTION 1: INMU (Thai FCD, INMU Mahidol) — 17 groups / 1,781 foods / 60,751 nutrient rows
-- =============================================================================

CREATE TABLE IF NOT EXISTS `foodcheck_food_groups` (
  `status`        CHAR(1) NOT NULL,
  `food_group_id` INT NOT NULL,
  `name_en`       VARCHAR(255) NOT NULL,
  `created_at`    DATETIME NOT NULL,
  `updated_at`    DATETIME NOT NULL,
  PRIMARY KEY (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `foodcheck_foods` (
  `id`              BIGINT UNSIGNED NOT NULL, -- preserved verbatim from INMU's internal id (not auto-increment) so nutrient rows can FK onto it unchanged
  `food_code`       VARCHAR(20) NULL,
  `status`          CHAR(1) NOT NULL,
  `food_group_id`   INT NOT NULL,
  `name_th`         VARCHAR(500) NULL,
  `name_en`         VARCHAR(500) NULL,
  `scientific_name` VARCHAR(255) NULL,
  `dbcode`          VARCHAR(20) NOT NULL DEFAULT 'STD',
  `created_at`      DATETIME NOT NULL,
  `updated_at`      DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_foodcheck_foods_status` (`status`),
  KEY `idx_foodcheck_foods_food_code` (`food_code`),
  KEY `idx_foodcheck_foods_name_th` (`name_th`),
  KEY `idx_foodcheck_foods_name_en` (`name_en`),
  CONSTRAINT `fk_foodcheck_foods_status` FOREIGN KEY (`status`)
    REFERENCES `foodcheck_food_groups` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `foodcheck_food_nutrients` (
  `food_id`       BIGINT UNSIGNED NOT NULL,
  `nutrient_name` VARCHAR(100) NOT NULL,
  `unit`          VARCHAR(20) NULL,
  `per_100g`      DECIMAL(14,4) NULL, -- NULL = missing (source had '-' or blank); see migration risk note above
  `deriv_by`      VARCHAR(30) NULL,   -- 'Analysed' | 'Calculated' | 'Not analysed'
  `n`             VARCHAR(20) NULL,   -- kept as text: source sometimes annotates this beyond a plain count
  `min_val`       DECIMAL(14,4) NULL,
  `max_val`       DECIMAL(14,4) NULL,
  `sd`            DECIMAL(14,4) NULL,
  `footnote`      VARCHAR(500) NULL,
  `last_updated`  VARCHAR(50) NULL,   -- kept as text: source format isn't a reliably parseable date
  `created_at`    DATETIME NOT NULL,
  `updated_at`    DATETIME NOT NULL,
  PRIMARY KEY (`food_id`, `nutrient_name`),
  KEY `idx_foodcheck_food_nutrients_name` (`nutrient_name`),
  CONSTRAINT `fk_foodcheck_food_nutrients_food_id` FOREIGN KEY (`food_id`)
    REFERENCES `foodcheck_foods` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- SECTION 2: กรมอนามัย (Anamai) — 1,484 foods / 24,364 nutrient rows
-- =============================================================================

CREATE TABLE IF NOT EXISTS `foodcheck_anamai_foods` (
  `fid`           VARCHAR(10) NOT NULL, -- zero-padded 5-digit id, or 'R...' for Branded Food Products
  `name_th`       VARCHAR(500) NULL,
  `name_en`       VARCHAR(500) NULL,
  `food_group_th` VARCHAR(255) NULL,
  `food_group_en` VARCHAR(255) NULL,
  `food_type`     VARCHAR(100) NULL,
  `created_at`    DATETIME NOT NULL,
  `updated_at`    DATETIME NOT NULL,
  PRIMARY KEY (`fid`),
  KEY `idx_foodcheck_anamai_foods_name_th` (`name_th`),
  KEY `idx_foodcheck_anamai_foods_name_en` (`name_en`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `foodcheck_anamai_nutrients` (
  `fid`           VARCHAR(10) NOT NULL,
  `category`      VARCHAR(50) NULL, -- 'Main nutrients' | 'Minerals' | 'Vitamins' | ...
  `nutrient_name` VARCHAR(100) NOT NULL,
  `amount`        DECIMAL(14,4) NULL,
  `unit`          VARCHAR(20) NULL,
  `created_at`    DATETIME NOT NULL,
  `updated_at`    DATETIME NOT NULL,
  PRIMARY KEY (`fid`, `nutrient_name`),
  KEY `idx_foodcheck_anamai_nutrients_name` (`nutrient_name`),
  CONSTRAINT `fk_foodcheck_anamai_nutrients_fid` FOREIGN KEY (`fid`)
    REFERENCES `foodcheck_anamai_foods` (`fid`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- SECTION 3: PD-critical nutrient config + cross-source name mapping
-- =============================================================================

-- The 6 nutrients PD (peritoneal dialysis) patients need surfaced, and their
-- Thai display labels — used to highlight rows regardless of which source
-- (INMU/Anamai) the food came from. Actual traffic-light risk thresholds
-- are NOT stored here (see internal/foodrisk).
CREATE TABLE IF NOT EXISTS `foodcheck_pd_nutrients` (
  `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `nutrient_name`   VARCHAR(100) NOT NULL, -- canonical name, matches foodcheck_food_nutrients.nutrient_name (INMU spelling)
  `display_name_th` VARCHAR(100) NOT NULL,
  `unit`            VARCHAR(20) NOT NULL,
  `sort_order`      INT NOT NULL DEFAULT 0,
  `created_at`      DATETIME NOT NULL,
  `updated_at`      DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_foodcheck_pd_nutrients_name` (`nutrient_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `foodcheck_pd_nutrients`
  (`nutrient_name`, `display_name_th`, `unit`, `sort_order`, `created_at`, `updated_at`)
VALUES
  ('Energy, by calculation', 'พลังงาน',      'kcal', 1, NOW(), NOW()),
  ('Protein, total',         'โปรตีน',        'g',    2, NOW(), NOW()),
  ('Phosphorus',             'ฟอสฟอรัส',      'mg',   3, NOW(), NOW()),
  ('Potassium',              'โพแทสเซียม',    'mg',   4, NOW(), NOW()),
  ('Sodium',                 'โซเดียม',       'mg',   5, NOW(), NOW()),
  ('Moisture',               'น้ำ/ความชื้น',  'g',    6, NOW(), NOW())
ON DUPLICATE KEY UPDATE `nutrient_name` = `nutrient_name`;

-- Anamai spells some of the 6 PD nutrients differently (e.g. 'Water' vs
-- INMU's 'Moisture'); this resolves either source's spelling to the
-- canonical name above so both sources can be highlighted the same way.
CREATE TABLE IF NOT EXISTS `foodcheck_nutrient_name_maps` (
  `source`         ENUM('thaifcd_inmu','thaifcd_anamai') NOT NULL,
  `source_name`    VARCHAR(100) NOT NULL,
  `canonical_name` VARCHAR(100) NOT NULL,
  `created_at`     DATETIME NOT NULL,
  `updated_at`     DATETIME NOT NULL,
  PRIMARY KEY (`source`, `source_name`),
  KEY `idx_foodcheck_nutrient_name_maps_canonical` (`canonical_name`),
  CONSTRAINT `fk_foodcheck_nutrient_name_maps_canonical` FOREIGN KEY (`canonical_name`)
    REFERENCES `foodcheck_pd_nutrients` (`nutrient_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `foodcheck_nutrient_name_maps`
  (`source`, `source_name`, `canonical_name`, `created_at`, `updated_at`)
VALUES
  ('thaifcd_inmu',   'Energy, by calculation', 'Energy, by calculation', NOW(), NOW()),
  ('thaifcd_inmu',   'Protein, total',         'Protein, total',         NOW(), NOW()),
  ('thaifcd_inmu',   'Phosphorus',             'Phosphorus',             NOW(), NOW()),
  ('thaifcd_inmu',   'Potassium',              'Potassium',              NOW(), NOW()),
  ('thaifcd_inmu',   'Sodium',                 'Sodium',                 NOW(), NOW()),
  ('thaifcd_inmu',   'Moisture',               'Moisture',               NOW(), NOW()),
  ('thaifcd_anamai', 'Energy',                 'Energy, by calculation', NOW(), NOW()),
  ('thaifcd_anamai', 'Total Energy',           'Energy, by calculation', NOW(), NOW()),
  ('thaifcd_anamai', 'Protein',                'Protein, total',         NOW(), NOW()),
  ('thaifcd_anamai', 'Phosphorus',             'Phosphorus',             NOW(), NOW()),
  ('thaifcd_anamai', 'Potassium',              'Potassium',              NOW(), NOW()),
  ('thaifcd_anamai', 'Sodium',                 'Sodium',                 NOW(), NOW()),
  ('thaifcd_anamai', 'Water',                  'Moisture',               NOW(), NOW())
ON DUPLICATE KEY UPDATE `canonical_name` = VALUES(`canonical_name`);

-- =============================================================================
-- SECTION 4: Recipe Builder — placeholder only, no endpoints yet (see
-- docs/foodcheck_survey.md — 0 rows in the source, never had a UI). Kept as
-- a migrated table so the shape exists before any feature work starts, and
-- tied to patient_profiles instead of a separate foodcheck user table.
-- =============================================================================

CREATE TABLE IF NOT EXISTS `foodcheck_recipes` (
  `id`                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `patient_profile_id`  BIGINT UNSIGNED NOT NULL,
  `name`                VARCHAR(255) NOT NULL,
  `description`         TEXT NULL,
  `servings`            DECIMAL(6,2) NOT NULL DEFAULT 1,
  `serving_unit`        VARCHAR(50) NOT NULL DEFAULT 'จาน',
  `is_public`           TINYINT(1) NOT NULL DEFAULT 0,
  `created_at`          DATETIME NOT NULL,
  `updated_at`          DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_foodcheck_recipes_patient_profile_id` (`patient_profile_id`),
  CONSTRAINT `fk_foodcheck_recipes_patient_profile_id` FOREIGN KEY (`patient_profile_id`)
    REFERENCES `patient_profiles` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `foodcheck_recipe_ingredients` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `recipe_id`  BIGINT UNSIGNED NOT NULL,
  `food_id`    BIGINT UNSIGNED NOT NULL, -- INMU only, same limitation as the source (see foodcheck_survey.md 4.2)
  `amount_g`   DECIMAL(10,2) NOT NULL,
  `note`       VARCHAR(255) NULL,
  `sort_order` INT NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_foodcheck_recipe_ingredients_recipe_id` (`recipe_id`),
  KEY `idx_foodcheck_recipe_ingredients_food_id` (`food_id`),
  CONSTRAINT `fk_foodcheck_recipe_ingredients_recipe_id` FOREIGN KEY (`recipe_id`)
    REFERENCES `foodcheck_recipes` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_foodcheck_recipe_ingredients_food_id` FOREIGN KEY (`food_id`)
    REFERENCES `foodcheck_foods` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- SECTION 5: Search history — new table, did not exist in the source system.
-- Records what a patient searched (and, if they opened it, which food) for
-- future personalization/SSO-aware features. No history existed to migrate.
-- =============================================================================

CREATE TABLE IF NOT EXISTS `foodcheck_search_history` (
  `id`                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `patient_profile_id` BIGINT UNSIGNED NOT NULL,
  `query`              VARCHAR(255) NOT NULL,
  `food_source`        ENUM('thaifcd_inmu','thaifcd_anamai') NULL, -- NULL if they searched but never opened a result
  `food_ref`           VARCHAR(20) NULL, -- foodcheck_foods.id or foodcheck_anamai_foods.fid, as text
  `searched_at`        DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_foodcheck_search_history_patient_profile_id` (`patient_profile_id`, `searched_at`),
  CONSTRAINT `fk_foodcheck_search_history_patient_profile_id` FOREIGN KEY (`patient_profile_id`)
    REFERENCES `patient_profiles` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- SECTION 6: Views — mirror the source's v_food_nutrient / v_pd_nutrient /
-- v_recipe_nutrition (see docs/foodcheck_survey.md section 2), minus the
-- USDA fallback branch (dropped — see note at top of this file).
-- =============================================================================

CREATE OR REPLACE VIEW `v_foodcheck_food_nutrients` AS
SELECT
    f.id            AS food_id,
    f.food_code,
    f.name_th,
    f.name_en,
    f.status        AS food_group,
    n.nutrient_name,
    n.unit,
    n.per_100g,
    n.deriv_by,
    CASE WHEN n.per_100g IS NULL
              OR LOWER(TRIM(COALESCE(n.deriv_by, ''))) = 'not analysed'
         THEN 1 ELSE 0
    END             AS is_missing
FROM `foodcheck_foods` f
JOIN `foodcheck_food_nutrients` n ON n.food_id = f.id;

CREATE OR REPLACE VIEW `v_foodcheck_pd_nutrients` AS
-- Source 1: INMU — nutrient_name already matches foodcheck_pd_nutrients, no mapping needed
SELECT
    CONCAT('thaifcd_inmu:', f.id) AS food_uid,
    'thaifcd_inmu'  AS source,
    f.food_code,
    f.name_th,
    f.name_en,
    pd.nutrient_name,
    pd.display_name_th,
    pd.unit,
    pd.sort_order,
    n.per_100g      AS value_per_100g,
    CASE WHEN n.per_100g IS NOT NULL THEN 'Thai FCD (INMU)' ELSE NULL END AS value_source
FROM `foodcheck_foods` f
CROSS JOIN `foodcheck_pd_nutrients` pd
LEFT JOIN `foodcheck_food_nutrients` n
       ON n.food_id = f.id AND n.nutrient_name = pd.nutrient_name

UNION ALL

-- Source 2: Anamai — resolve each source_name to the canonical name first
-- (an_canon) before joining to pd, same reasoning as the source system:
-- one canonical name can have multiple source_name spellings, so joining
-- the mapping table directly inside the CROSS JOIN would fan out duplicate
-- rows.
SELECT
    CONCAT('thaifcd_anamai:', af.fid) AS food_uid,
    'thaifcd_anamai' AS source,
    af.fid          AS food_code,
    af.name_th,
    af.name_en,
    pd.nutrient_name,
    pd.display_name_th,
    pd.unit,
    pd.sort_order,
    an_canon.amount AS value_per_100g,
    CASE WHEN an_canon.amount IS NOT NULL THEN 'Thai FCD (Anamai)' ELSE NULL END AS value_source
FROM `foodcheck_anamai_foods` af
CROSS JOIN `foodcheck_pd_nutrients` pd
LEFT JOIN (
    SELECT an.fid, map.canonical_name, an.amount
    FROM `foodcheck_anamai_nutrients` an
    JOIN `foodcheck_nutrient_name_maps` map
         ON map.source = 'thaifcd_anamai' AND map.source_name = an.nutrient_name
) an_canon
       ON an_canon.fid = af.fid AND an_canon.canonical_name = pd.nutrient_name;

CREATE OR REPLACE VIEW `v_foodcheck_recipe_nutrition` AS
SELECT
    ri.recipe_id,
    r.name              AS recipe_name,
    r.patient_profile_id,
    r.servings,
    pd.nutrient_name,
    pd.display_name_th,
    pd.unit,
    pd.sort_order,
    SUM(n.per_100g * ri.amount_g / 100.0)             AS total_value,
    SUM(n.per_100g * ri.amount_g / 100.0) / r.servings AS per_serving_value
FROM `foodcheck_recipe_ingredients` ri
JOIN `foodcheck_recipes` r    ON r.id = ri.recipe_id
CROSS JOIN `foodcheck_pd_nutrients` pd
LEFT JOIN `foodcheck_food_nutrients` n
       ON n.food_id = ri.food_id AND n.nutrient_name = pd.nutrient_name
GROUP BY ri.recipe_id, pd.nutrient_name;
