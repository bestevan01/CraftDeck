-- Singleton row (id always 1), same pattern as network_settings (0002).
-- cooler_checked_at doubles as the "has the one-shot detection already run"
-- flag: NULL means craftdeckd hasn't run DetectActiveCooler yet on this
-- install (see cmd/craftdeckd/main.go), so a fresh install and the first
-- restart after upgrading to a build that has this feature both naturally
-- trigger exactly one detection pass; every later restart sees a non-NULL
-- timestamp and skips it (the test spins the fan to full speed, which
-- isn't something to repeat on every restart).
CREATE TABLE hardware_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  cooler_detected INTEGER NOT NULL DEFAULT 0,
  cooler_checked_at TEXT,
  overclock_enabled INTEGER NOT NULL DEFAULT 0,
  overclock_preset TEXT NOT NULL DEFAULT '',
  overclock_arm_freq INTEGER,
  overclock_over_voltage INTEGER,
  overclock_applied_at TEXT,
  last_benchmark_result TEXT NOT NULL DEFAULT '',
  last_benchmark_at TEXT
);

INSERT INTO hardware_settings (id) VALUES (1);
