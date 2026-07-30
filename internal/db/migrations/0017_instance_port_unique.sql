-- nextFreeGamePort (handlers_instance.go) picks a port by scanning the
-- instances table and then, separately, inserting a row with it -- a
-- check-then-use gap. Two creation requests arriving close together could
-- both be handed the same port, and since rcon_port is derived as
-- game_port + 10000, their RCON ports collide too (the exact combination a
-- comment in handleCreateInstance records as having caused an endless
-- connect/auth-fail/reconnect loop on real hardware).
--
-- A unique index makes the database itself the arbiter: the loser of that
-- race fails its INSERT instead of silently sharing a port. SQLite can't
-- add a UNIQUE constraint to an existing column via ALTER TABLE, so this is
-- expressed as a unique index -- functionally equivalent for enforcement.
--
-- rcon_port gets the same treatment, but only for rows that actually use it:
-- the Velocity proxy stores rcon_port = 0 (it has no RCON in this MVP), and
-- a plain unique index would let exactly one such row exist. The partial
-- index skips those rows entirely.
CREATE UNIQUE INDEX idx_instances_game_port ON instances(game_port);
CREATE UNIQUE INDEX idx_instances_rcon_port ON instances(rcon_port) WHERE rcon_port != 0;
