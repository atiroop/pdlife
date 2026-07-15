-- Migration: Admin User Management (account-level only).
-- admin_action_logs is the mandatory audit trail for every action an admin
-- performs against another user's account — no admin action may skip it
-- (enforced in internal/handler/admin_users.go: the action and its log row
-- are written in one transaction). users.suspended_at/suspended_reason back
-- the "ระงับบัญชี" action; non-NULL suspended_at blocks login the same way
-- account_deletion_requested_at does.

CREATE TABLE `admin_action_logs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `admin_id` BIGINT UNSIGNED NOT NULL,
  `target_user_id` BIGINT UNSIGNED NOT NULL,
  `action` ENUM('manual_verify_email','unlock_account','suspend_account','unsuspend_account') NOT NULL,
  `reason` TEXT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_admin_action_logs_target` (`target_user_id`, `created_at`),
  KEY `idx_admin_action_logs_admin` (`admin_id`),
  CONSTRAINT `fk_admin_action_logs_admin`
    FOREIGN KEY (`admin_id`) REFERENCES `users` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_admin_action_logs_target`
    FOREIGN KEY (`target_user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE `users`
  ADD COLUMN `suspended_at` DATETIME NULL DEFAULT NULL AFTER `terms_accepted_version`,
  ADD COLUMN `suspended_reason` TEXT NULL AFTER `suspended_at`;
