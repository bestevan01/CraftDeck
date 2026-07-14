-- The operator asked to merge the separate web-UI/game-port WAN toggles
-- (FR-21/FR-25) into a single "외부 접속 허용" switch that also drives
-- per-instance game-port forwarding as instances start/stop (see
-- ReconcileGamePorts in internal/api). wan_game_enabled was never actually
-- exposed through any endpoint before this, so it's left in place unused
-- rather than dropped -- harmless dead column, safer than an ALTER TABLE
-- DROP COLUMN for no real benefit.
ALTER TABLE network_settings RENAME COLUMN wan_web_enabled TO wan_enabled;
