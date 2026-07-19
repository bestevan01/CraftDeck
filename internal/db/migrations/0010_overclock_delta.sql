-- Pi 5's firmware uses over_voltage_delta (microvolts), not the legacy
-- over_voltage integer-step key earlier Pi models used -- confirmed on
-- real hardware (see internal/hardware/overclock.go's Preset doc comment).
-- 0009 shipped with the wrong key's column name before this was caught;
-- rename rather than add a new column since no real config has diverged
-- from this yet.
ALTER TABLE hardware_settings RENAME COLUMN overclock_over_voltage TO overclock_over_voltage_delta;
