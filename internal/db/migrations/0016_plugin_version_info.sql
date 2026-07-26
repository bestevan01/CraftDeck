-- Records which Modrinth version/channel is actually installed (as opposed
-- to just modrinth_version_id, an opaque ID the UI can't display on its
-- own) -- lets the installed-mods list show "1.2.0-beta.3 (beta)" instead
-- of nothing at all. NULL for anything uploaded directly (no Modrinth
-- version to record) and, for pre-existing rows installed before this
-- migration, until the operator reinstalls that mod.
ALTER TABLE plugins ADD COLUMN version_number TEXT;
ALTER TABLE plugins ADD COLUMN version_channel TEXT;
