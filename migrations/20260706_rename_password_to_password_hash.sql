-- Migration: rename users.password -> users.password_hash
-- Reason: column stores a bcrypt hash only, never plain text — name it
-- accordingly so that's obvious without needing a comment or extra context.
-- See docs/schema_spec.md.

ALTER TABLE `users` RENAME COLUMN `password` TO `password_hash`;
