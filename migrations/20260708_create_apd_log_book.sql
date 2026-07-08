-- Migration: apd_prescriptions, apd_log_entries
-- APD Log Book — migrated from the legacy Next.js app at apd.jocky.website
-- (Prisma models APDPrescription / APDDailyLog). Field set mirrors the
-- source exactly; see docs/schema_spec.md for the project-wide pattern.

CREATE TABLE IF NOT EXISTS `apd_prescriptions` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `patient_profile_id` BIGINT UNSIGNED NOT NULL,
  `name` VARCHAR(255) NOT NULL DEFAULT 'โปรไฟล์น้ำยาเริ่มต้น',
  `solution_bag_1` VARCHAR(100) NOT NULL,
  `solution_bag_2` VARCHAR(100) NOT NULL,
  `total_volume_ml` INT NOT NULL,
  `therapy_time_minutes` INT NOT NULL,
  `fill_volume_ml` INT NOT NULL,
  `cycles` INT NOT NULL,
  `dwell_time_minutes` INT NOT NULL,
  `last_fill_ml` INT NULL DEFAULT 0,
  `manual_exchange` TEXT NULL,
  `is_default_profile` TINYINT(1) NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_apd_prescriptions_patient_profile_id` (`patient_profile_id`),
  KEY `idx_apd_prescriptions_is_default_profile` (`is_default_profile`),
  CONSTRAINT `fk_apd_prescriptions_patient_profile_id` FOREIGN KEY (`patient_profile_id`)
    REFERENCES `patient_profiles` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `apd_log_entries` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `patient_profile_id` BIGINT UNSIGNED NOT NULL,
  `entry_date` DATE NOT NULL,
  `treatment_start_time` VARCHAR(20) NOT NULL,
  `weight_kg` DECIMAL(5,2) NOT NULL,
  `bp_systolic` SMALLINT NOT NULL,
  `bp_diastolic` SMALLINT NOT NULL,
  `pulse` SMALLINT NOT NULL,
  `blood_glucose_mg_dl` SMALLINT NULL DEFAULT NULL,
  `i_drain_volume_ml` INT NOT NULL,
  `total_uf_ml` INT NOT NULL,
  `urine_avg_day_ml` INT NOT NULL,
  `drainage_appearance` VARCHAR(50) NULL DEFAULT NULL,
  `remark` TEXT NULL,
  `prescription_id` BIGINT UNSIGNED NULL DEFAULT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_apd_log_entries_patient_date` (`patient_profile_id`, `entry_date`),
  KEY `idx_apd_log_entries_prescription_id` (`prescription_id`),
  CONSTRAINT `fk_apd_log_entries_patient_profile_id` FOREIGN KEY (`patient_profile_id`)
    REFERENCES `patient_profiles` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_apd_log_entries_prescription_id` FOREIGN KEY (`prescription_id`)
    REFERENCES `apd_prescriptions` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
