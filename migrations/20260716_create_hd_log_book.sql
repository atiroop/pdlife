-- Migration: hd_log_entries
-- HD Log Book — new feature, no legacy data to migrate. One row per
-- dialysis session (patient typically logs 2-3 sessions/week), matching
-- APD's one-row-per-day shape rather than CAPD's per-cycle shape. See
-- docs/schema_spec.md for the project-wide pattern.
--
-- uf_removed_ml is a TOTAL volume removed during the session (computed
-- server-side = (pre_dialysis_weight_kg - post_dialysis_weight_kg) * 1000),
-- NOT an hourly ultrafiltration rate.

CREATE TABLE IF NOT EXISTS `hd_log_entries` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `patient_profile_id` BIGINT UNSIGNED NOT NULL,
  `log_date` DATE NOT NULL,
  `dry_weight_kg` DECIMAL(5,2) NOT NULL,
  `pre_dialysis_weight_kg` DECIMAL(5,2) NOT NULL,
  `post_dialysis_weight_kg` DECIMAL(5,2) NOT NULL,
  `pre_dialysis_bp_systolic` SMALLINT NOT NULL,
  `pre_dialysis_bp_diastolic` SMALLINT NOT NULL,
  `post_dialysis_bp_systolic` SMALLINT NOT NULL,
  `post_dialysis_bp_diastolic` SMALLINT NOT NULL,
  `uf_removed_ml` INT NOT NULL COMMENT 'computed server-side = (pre_dialysis_weight_kg - post_dialysis_weight_kg) * 1000, total volume not hourly rate, not user-entered',
  `notes` TEXT NULL DEFAULT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_hd_log_entries_patient_date` (`patient_profile_id`, `log_date`),
  KEY `idx_hd_log_entries_patient_date` (`patient_profile_id`, `log_date`),
  CONSTRAINT `fk_hd_log_entries_patient_profile_id` FOREIGN KEY (`patient_profile_id`)
    REFERENCES `patient_profiles` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
