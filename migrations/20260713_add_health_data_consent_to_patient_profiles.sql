-- Migration: add explicit health-data consent tracking to patient_profiles
-- (PDPA section 26 requires separate, explicit consent for sensitive
-- personal data such as dialysis logs, weight, blood pressure). Nullable
-- timestamp: NULL means consent has not been given (or was withdrawn).

ALTER TABLE `patient_profiles`
  ADD COLUMN `health_data_consent_at` DATETIME NULL DEFAULT NULL AFTER `profile_completed_at`,
  ADD COLUMN `health_data_consent_version` VARCHAR(50) NULL DEFAULT NULL AFTER `health_data_consent_at`;
