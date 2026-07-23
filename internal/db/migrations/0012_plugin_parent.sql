-- Tracks which installed plugin/mod triggered a dependency's auto-install
-- (FR-6c), so the UI can group dependencies under the plugin the operator
-- actually searched for instead of showing them as an undifferentiated
-- flat list. NULL for anything installed directly (search or upload).
--
-- ON DELETE SET NULL rather than CASCADE: deleting the parent shouldn't
-- delete its dependency files too (another plugin might still need them,
-- or the operator just wants the one plugin gone) -- it just becomes an
-- unparented entry the UI groups under "other dependencies" instead.
ALTER TABLE plugins ADD COLUMN parent_plugin_id TEXT REFERENCES plugins(id) ON DELETE SET NULL;
