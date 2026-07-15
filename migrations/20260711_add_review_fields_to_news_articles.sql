-- Migration: add moderation-review fields to news_articles
-- Supports the /admin/content-queue approve/reject workflow — every
-- status change away from 'pending' must record who did it and when.

ALTER TABLE `news_articles`
  ADD COLUMN `reviewed_by` BIGINT UNSIGNED NULL AFTER `status`,
  ADD COLUMN `reviewed_at` DATETIME NULL AFTER `reviewed_by`,
  ADD CONSTRAINT `fk_news_articles_reviewed_by` FOREIGN KEY (`reviewed_by`)
    REFERENCES `users` (`id`) ON DELETE SET NULL;
