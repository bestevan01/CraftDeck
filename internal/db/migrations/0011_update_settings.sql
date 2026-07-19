-- Singleton row (id always 1), same pattern as network_settings (0002) and
-- hardware_settings (0009). cached_latest_version/last_checked_at let
-- handleCraftdeckVersion honor check_frequency without hitting the apt
-- repo on every page load -- see internal/api/handlers_system.go.
CREATE TABLE update_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  channel TEXT NOT NULL DEFAULT 'stable',
  check_frequency TEXT NOT NULL DEFAULT 'every_visit',
  cached_latest_version TEXT NOT NULL DEFAULT '',
  last_checked_at TEXT
);

INSERT INTO update_settings (id) VALUES (1);
