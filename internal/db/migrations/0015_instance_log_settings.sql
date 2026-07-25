-- Per-instance controls for gamelog's on-disk console capture (separate
-- from journald's own system-wide retention, see internal/gamelog): whether
-- to capture at all, and how captured log files get cleaned up once
-- they've accumulated. Defaults match what most operators actually want
-- (capture on, 30 days of history) rather than opting everyone into a new
-- disk-usage habit silently -- but this is exactly what backfills every
-- pre-existing instance to "on" too, which is the intended behavior here
-- (the feature this supports is "keep console history since the server
-- started", not "off unless asked").
ALTER TABLE instances ADD COLUMN log_storage_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE instances ADD COLUMN log_retention_mode TEXT NOT NULL DEFAULT 'age'
  CHECK (log_retention_mode IN ('unlimited', 'age', 'size'));
ALTER TABLE instances ADD COLUMN log_retention_days INTEGER NOT NULL DEFAULT 30;
ALTER TABLE instances ADD COLUMN log_retention_max_mb INTEGER NOT NULL DEFAULT 500;
