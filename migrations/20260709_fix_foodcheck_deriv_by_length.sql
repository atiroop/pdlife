-- Migration: widen foodcheck_food_nutrients.deriv_by
--
-- 20260709_create_foodcheck.sql sized this at VARCHAR(30), guessed from the
-- short examples in docs/foodcheck_survey.md ('Analysed' | 'Calculated' |
-- 'Not analysed'). The real source data also contains long free-text
-- explanations, e.g. 'Assumed zero (Insignificant amount or not naturally
-- occurring in a food, such as fiber in meat)' (95 chars) — discovered when
-- cmd/migrate_foodcheck hit "Data too long for column 'deriv_by'" partway
-- through the real import. VARCHAR(150) leaves headroom above the observed
-- max (95).
--
-- Not editing 20260709_create_foodcheck.sql itself: it already ran on
-- production (see migrations/README.md discipline — never edit an applied
-- migration, write a new one).

ALTER TABLE `foodcheck_food_nutrients`
  MODIFY COLUMN `deriv_by` VARCHAR(150) NULL;
