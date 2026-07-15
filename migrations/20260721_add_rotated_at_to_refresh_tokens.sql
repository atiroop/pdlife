-- Migration: distinguish a rotated refresh token from a deliberately
-- revoked one.
--
-- revoked_at alone cannot tell the two apart, and they need opposite
-- treatment. Rotation happens constantly and normally (every time an
-- expired access token is renewed), and sibling requests already in flight
-- still carry the token that was just rotated away — rejecting those logs
-- the patient out at random. Logout, password change/reset, account
-- deletion and admin suspension also set revoked_at, and those must take
-- effect immediately.
--
-- rotated_at is set only by the rotation path in internal/handler/session.go,
-- always in the same UPDATE as revoked_at. A token with revoked_at set and
-- rotated_at NULL was killed on purpose.
--
-- See docs/schema_spec.md.

ALTER TABLE `refresh_tokens`
  ADD COLUMN `rotated_at` DATETIME NULL DEFAULT NULL AFTER `revoked_at`;
