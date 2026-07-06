-- Migration: users, email_verifications, patient_profiles
-- See docs/schema_spec.md and docs/auth_flow_spec.md for the source of truth.

CREATE TABLE IF NOT EXISTS `users` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `email` VARCHAR(255) NOT NULL,
  `password` VARCHAR(255) NOT NULL,
  `nickname` VARCHAR(100) NOT NULL,
  `role` ENUM('Admin','Member','Unverified') NOT NULL DEFAULT 'Unverified',
  `is_active` TINYINT(1) NOT NULL DEFAULT 1,
  `email_verified_at` DATETIME NULL DEFAULT NULL,
  `last_login_at` DATETIME NULL DEFAULT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  `deleted_at` DATETIME NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_users_email` (`email`),
  KEY `idx_users_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `email_verifications` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `token_hash` CHAR(64) NOT NULL,
  `expires_at` DATETIME NOT NULL,
  `used_at` DATETIME NULL DEFAULT NULL,
  `created_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_email_verifications_token_hash` (`token_hash`),
  KEY `idx_email_verifications_user_id` (`user_id`),
  CONSTRAINT `fk_email_verifications_user_id` FOREIGN KEY (`user_id`)
    REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `patient_profiles` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `treatment_type` ENUM('CAPD','APD','HD') NULL DEFAULT NULL,
  `hospital_name` VARCHAR(255) NULL DEFAULT NULL,
  `coverage_type` ENUM('บัตรทอง','ประกันสังคม','ข้าราชการ','อื่นๆ') NULL DEFAULT NULL,
  `profile_completed_at` DATETIME NULL DEFAULT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_patient_profiles_user_id` (`user_id`),
  CONSTRAINT `fk_patient_profiles_user_id` FOREIGN KEY (`user_id`)
    REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
