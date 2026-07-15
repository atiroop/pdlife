-- Migration: lab_results
-- Lab Results — new feature, no legacy data to migrate. One row per test
-- date (like hd_log_entries/apd_log_entries), but unlike those, almost
-- every column is nullable: the patient's actual lab panel doesn't test
-- everything on the same visit (some values are drawn every 3 months,
-- others every 6-12 months), so a row typically has only a subset filled
-- in. Only patient_profile_id and log_date are required.
--
-- kt_v_value/urr/npcr are HD-specific (only shown on the form when
-- patient_profiles.treatment_type = 'HD') but stored on every patient's
-- row regardless of treatment type, same as how other nullable columns
-- work here — simplest schema, the application layer decides what to show.

CREATE TABLE IF NOT EXISTS `lab_results` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `patient_profile_id` BIGINT UNSIGNED NOT NULL,
  `log_date` DATE NOT NULL,

  -- ตรวจทุก 3 เดือน
  `hct` DECIMAL(5,2) NULL DEFAULT NULL,
  `hb` DECIMAL(4,2) NULL DEFAULT NULL,
  `wbc` INT NULL DEFAULT NULL,
  `platelet_count` INT NULL DEFAULT NULL,
  `bun` DECIMAL(5,2) NULL DEFAULT NULL,
  `cr` DECIMAL(4,2) NULL DEFAULT NULL,
  `na` DECIMAL(5,2) NULL DEFAULT NULL,
  `k` DECIMAL(4,2) NULL DEFAULT NULL,
  `co2` DECIMAL(5,2) NULL DEFAULT NULL,
  `ca` DECIMAL(5,2) NULL DEFAULT NULL,
  `po4` DECIMAL(4,2) NULL DEFAULT NULL,
  `albumin` DECIMAL(4,2) NULL DEFAULT NULL,
  `kt_v_value` DECIMAL(4,2) NULL DEFAULT NULL COMMENT 'measured value only, never auto-classified — see internal/labrange doc comment',
  `urr` DECIMAL(5,2) NULL DEFAULT NULL COMMENT 'HD only',
  `npcr` DECIMAL(4,2) NULL DEFAULT NULL COMMENT 'HD only',

  -- ตรวจทุก 6 เดือน / 1 ปี
  `fbs` DECIMAL(6,2) NULL DEFAULT NULL,
  `hba1c` DECIMAL(4,2) NULL DEFAULT NULL,
  `uric_acid` DECIMAL(4,2) NULL DEFAULT NULL,
  `pth` DECIMAL(7,2) NULL DEFAULT NULL,
  `ferritin` DECIMAL(8,2) NULL DEFAULT NULL,
  `serum_iron` DECIMAL(6,2) NULL DEFAULT NULL,
  `tibc` DECIMAL(6,2) NULL DEFAULT NULL,
  `t_sat_percent` DECIMAL(5,2) NULL DEFAULT NULL,
  `chol` DECIMAL(6,2) NULL DEFAULT NULL,
  `hdl` DECIMAL(5,2) NULL DEFAULT NULL,
  `ldl` DECIMAL(6,2) NULL DEFAULT NULL,
  `hbsag` ENUM('negative','positive') NULL DEFAULT NULL,
  `hbsab` ENUM('negative','positive') NULL DEFAULT NULL,
  `anti_hcv` ENUM('negative','positive') NULL DEFAULT NULL,
  `anti_hiv` ENUM('negative','positive') NULL DEFAULT NULL,
  `cxr_finding` TEXT NULL DEFAULT NULL,
  `ekg_finding` TEXT NULL DEFAULT NULL,

  `notes` TEXT NULL DEFAULT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_lab_results_patient_date` (`patient_profile_id`, `log_date`),
  KEY `idx_lab_results_patient_date` (`patient_profile_id`, `log_date`),
  CONSTRAINT `fk_lab_results_patient_profile_id` FOREIGN KEY (`patient_profile_id`)
    REFERENCES `patient_profiles` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
