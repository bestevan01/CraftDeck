-- FR-21/FR-25: a single persisted toggle per port-type ("게임 포트" vs
-- "관리 웹 UI 포트"), replacing the source-IP heuristic requireAuth used as
-- a stopgap (see internal/api/router.go). Singleton row (id is always 1) --
-- there's exactly one craftdeckd instance managing exactly one router.
CREATE TABLE network_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  wan_web_enabled INTEGER NOT NULL DEFAULT 0,
  wan_game_enabled INTEGER NOT NULL DEFAULT 0
);

INSERT INTO network_settings (id, wan_web_enabled, wan_game_enabled) VALUES (1, 0, 0);
