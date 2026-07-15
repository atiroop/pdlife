-- Migration: PDLife Editorial Articles — admin-authored rich-text
-- articles with image/video media (uploaded to R2, see internal/r2store),
-- sanitized HTML content (see internal/sanitize).

CREATE TABLE IF NOT EXISTS `editorial_articles` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `author_id` BIGINT UNSIGNED NOT NULL,
  `title` VARCHAR(255) NOT NULL,
  `slug` VARCHAR(255) NOT NULL,
  `content_html` MEDIUMTEXT NOT NULL,
  `cover_image_url` VARCHAR(500) NULL DEFAULT NULL,
  `status` ENUM('draft','published') NOT NULL DEFAULT 'draft',
  `published_at` DATETIME NULL DEFAULT NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_editorial_articles_slug` (`slug`),
  KEY `idx_editorial_articles_status_published_at` (`status`, `published_at`),
  CONSTRAINT `fk_editorial_articles_author_id` FOREIGN KEY (`author_id`)
    REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
