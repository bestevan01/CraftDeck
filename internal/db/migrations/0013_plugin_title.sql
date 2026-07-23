-- Modrinth's display title (e.g. "Sodium") for a plugin/mod, separate from
-- its on-disk jar filename (e.g. "sodium-fabric-0.6.13.jar") -- lets the UI
-- show the readable name. NULL for direct .jar uploads, which have no
-- Modrinth project to look a title up from; the UI falls back to filename.
ALTER TABLE plugins ADD COLUMN title TEXT;
