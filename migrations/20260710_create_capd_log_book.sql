-- Migration: capd_log_entries
-- CAPD Log Book — new feature, no legacy data to migrate. Unlike APD (one
-- entry per patient per day), CAPD logs one row per exchange *cycle*
-- (typically 1-5 per day), so the uniqueness constraint is per
-- (patient, date, cycle) rather than per (patient, date). See
-- docs/schema_spec.md for the project-wide pattern.

CREATE TABLE IF NOT EXISTS `capd_log_entries` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `patient_profile_id` BIGINT UNSIGNED NOT NULL,
  `log_date` DATE NOT NULL,
  `cycle_number` TINYINT UNSIGNED NOT NULL,
  `dextrose_concentration` DECIMAL(4,2) NOT NULL,
  `fill_start_time` VARCHAR(20) NOT NULL,
  `fill_end_time` VARCHAR(20) NOT NULL,
  `fill_volume_ml` INT NOT NULL,
  `drain_start_time` VARCHAR(20) NOT NULL,
  `drain_end_time` VARCHAR(20) NOT NULL,
  `drain_volume_ml` INT NOT NULL,
  `uf_volume_ml` INT NOT NULL COMMENT 'computed server-side = drain_volume_ml - fill_volume_ml, not user-entered',
  `dialysate_appearance` ENUM('clear','cloudy','bloody') NOT NULL,
  `weight_kg` DECIMAL(5,2) NOT NULL,
  `bp_systolic` SMALLINT NOT NULL,
  `bp_diastolic` SMALLINT NOT NULL,
  `urine_output_ml` INT NULL DEFAULT NULL COMMENT 'recorded once per day, tied to the last cycle of that log_date',
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_capd_log_entries_patient_date_cycle` (`patient_profile_id`, `log_date`, `cycle_number`),
  KEY `idx_capd_log_entries_patient_date` (`patient_profile_id`, `log_date`),
  CONSTRAINT `fk_capd_log_entries_patient_profile_id` FOREIGN KEY (`patient_profile_id`)
    REFERENCES `patient_profiles` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
