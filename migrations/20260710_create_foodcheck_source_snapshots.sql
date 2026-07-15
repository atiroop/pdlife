-- Migration: foodcheck_source_snapshots
-- Backs cmd/foodcheck_diffcheck (Phase 3 of the Food Check port — see
-- docs/foodcheck_survey.md and internal/foodrisk for Phases 1-2). Stores
-- one row per monthly run per source, purely for drift detection against
-- the upstream INMU/Anamai websites. Never written to by the web app —
-- only by the standalone cmd/foodcheck_diffcheck tool, and never used to
-- auto-populate foodcheck_foods/foodcheck_anamai_foods (that stays a
-- manual, human-reviewed decision every time — see the tool's doc comment).

CREATE TABLE IF NOT EXISTS `foodcheck_source_snapshots` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `source`        ENUM('inmu','anamai') NOT NULL,
  `item_count`    INT NOT NULL,
  `content_hash`  CHAR(64) NOT NULL,       -- SHA-256 hex of the sorted list of item ids seen this run
  `checked_at`    DATETIME NOT NULL,
  `raw_snapshot`  LONGTEXT NULL,           -- JSON: sorted item ids, kept for debugging what changed
  `created_at`    DATETIME NOT NULL,
  `updated_at`    DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_foodcheck_source_snapshots_source_checked` (`source`, `checked_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
