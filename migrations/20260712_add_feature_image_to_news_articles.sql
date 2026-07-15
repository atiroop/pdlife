-- Migration: add AI-generated feature image fields to news_articles
-- See docs/schema_spec.md for the generation pipeline this supports.

ALTER TABLE `news_articles`
  ADD COLUMN `feature_image_url` VARCHAR(500) NULL AFTER `credit_url`,
  ADD COLUMN `feature_image_status` ENUM('pending','generated','failed') NOT NULL DEFAULT 'pending' AFTER `feature_image_url`;
