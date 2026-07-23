-- plugins.parent_plugin_id (0012_plugin_parent.sql) can only ever record one
-- parent, so a dependency shared by several mods (Fabric API being needed by
-- half a dozen Fabric mods is the common case) only ever showed up grouped
-- under whichever mod happened to trigger its install first -- the others
-- silently required it with no visible relationship at all. This table
-- records every (parent, dependency) edge instead of just the first one.
--
-- ON DELETE CASCADE on both sides is correct here specifically because a row
-- in this table is just an edge, not the dependency's own install record --
-- deleting either plugin should just drop the now-meaningless edge, not
-- touch the other plugin (that's still governed by parent_plugin_id's own
-- ON DELETE SET NULL on the plugins table itself).
CREATE TABLE plugin_dependencies (
  parent_plugin_id TEXT NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
  dependency_plugin_id TEXT NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
  PRIMARY KEY (parent_plugin_id, dependency_plugin_id)
);

-- Backfill the one edge each existing dependency already recorded via
-- parent_plugin_id, so upgrading doesn't lose the relationships already known.
INSERT INTO plugin_dependencies (parent_plugin_id, dependency_plugin_id)
SELECT parent_plugin_id, id FROM plugins WHERE parent_plugin_id IS NOT NULL;
