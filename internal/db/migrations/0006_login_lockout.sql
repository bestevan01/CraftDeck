-- FR-35: tracks consecutive login failures per account so repeated bad
-- attempts get locked out/rate-limited (FR-33(b)'s auto-stricter defaults
-- once WAN-exposed apply on top of this same mechanism).
ALTER TABLE users ADD COLUMN failed_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN locked_until TEXT;
