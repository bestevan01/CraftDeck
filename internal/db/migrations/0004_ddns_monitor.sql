-- FR-26f: persists whether the last periodic check found the monitor-only
-- provider's hostname (e.g. ipTime) pointing at a different IP than the
-- router's actual current WAN IP, so the frontend can show an alert
-- without doing a live DNS lookup on every page load.
ALTER TABLE ddns_configs ADD COLUMN mismatch_detected INTEGER NOT NULL DEFAULT 0;
