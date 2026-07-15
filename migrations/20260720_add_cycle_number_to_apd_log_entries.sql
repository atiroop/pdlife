-- Migration: allow multiple APD log entries per day (รอบที่ 1-6).
-- Real patients do several exchanges in one day (see the paper log book
-- example: 4-5 rounds/day with per-round fill/drain volumes), but the
-- original unique key allowed only one row per (patient, date). This adds
-- cycle_number — same model as capd_log_entries — and widens the unique
-- key to (patient, date, cycle). Existing rows become round 1 of their day.

ALTER TABLE `apd_log_entries`
  ADD COLUMN `cycle_number` TINYINT UNSIGNED NOT NULL DEFAULT 1 AFTER `entry_date`,
  DROP INDEX `uq_apd_log_entries_patient_date`,
  ADD UNIQUE KEY `uq_apd_log_entries_patient_date_cycle` (`patient_profile_id`, `entry_date`, `cycle_number`);
