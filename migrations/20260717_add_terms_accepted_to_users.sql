-- Migration: Terms of Service scroll-to-accept tracking on /register.
-- terms_accepted_version stores the "ปรับปรุงล่าสุด" date of /terms at the
-- moment of acceptance (handler.LegalContentUpdatedDate) — same pattern as
-- patient_profiles.health_data_consent_version — so a future revision of
-- the terms just needs a new constant, not a text diff, to tell which
-- users accepted which revision.

ALTER TABLE `users`
  ADD COLUMN `terms_accepted_at` DATETIME NULL DEFAULT NULL AFTER `account_deletion_requested_at`,
  ADD COLUMN `terms_accepted_version` VARCHAR(50) NULL DEFAULT NULL AFTER `terms_accepted_at`;
