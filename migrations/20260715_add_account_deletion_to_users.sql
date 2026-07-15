-- Migration: account deletion request tracking (PDPA right to erasure).
-- NULL = no pending deletion. Set once when a user confirms account
-- deletion from /profile; cmd/purge_deleted_accounts hard-deletes/
-- anonymizes 90 days after this timestamp. Login is blocked while set
-- (see internal/handler/login.go).

ALTER TABLE `users`
  ADD COLUMN `account_deletion_requested_at` DATETIME NULL DEFAULT NULL AFTER `last_login_at`;
