-- Migration: news_articles
-- Phase 4 News & Research — first source wired up is PubMed/NCBI
-- E-utilities (see docs/news_sources_survey.md for why: it's the only
-- source surveyed with an unambiguous green light for automated fetch +
-- AI summarization; kidney.org and healio.com explicitly forbid it in
-- their ToS). `source`/`external_id` are generic so nephrothai.org (or
-- any future source) can reuse this same table.

CREATE TABLE IF NOT EXISTS `news_articles` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `source` VARCHAR(50) NOT NULL COMMENT 'e.g. pubmed, nephrothai',
  `external_id` VARCHAR(100) NOT NULL COMMENT 'PMID for pubmed; source-specific id/slug otherwise',
  `title` VARCHAR(500) NOT NULL COMMENT 'original title, source language',
  `title_th` VARCHAR(500) NOT NULL COMMENT 'AI-translated Thai title',
  `summary_th` TEXT NOT NULL COMMENT 'AI-generated Thai summary (3-5 sentences) grounded strictly in the source abstract/content — never a verbatim copy of copyrighted source text',
  `content_html` MEDIUMTEXT NULL COMMENT 'full HTML body, only for sources with legitimate reuse permission for full content (see per-source credit terms) — always NULL for pubmed (abstract-only, fair-use safe)',
  `journal_name` VARCHAR(255) NULL,
  `published_at` DATE NULL COMMENT 'original publication date reported by the source',
  `credit_source_name` VARCHAR(255) NOT NULL COMMENT 'attribution text shown to the reader alongside the summary',
  `credit_url` VARCHAR(500) NOT NULL COMMENT 'link back to the original source — always shown, never omitted',
  `status` ENUM('pending','published','rejected') NOT NULL DEFAULT 'pending' COMMENT 'pending = ingested but awaiting human review before a patient ever sees it',
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_news_articles_source_external_id` (`source`, `external_id`),
  KEY `idx_news_articles_status` (`status`),
  KEY `idx_news_articles_published_at` (`published_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
