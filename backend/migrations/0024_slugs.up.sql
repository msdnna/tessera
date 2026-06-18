-- Human-readable URL identifiers. Boards and notes appear in URLs (tasks use
-- their per-workspace number), so they get a slug. Columns default to '' and are
-- backfilled at startup (transliteration happens in Go); new/renamed rows get a
-- slug in the handler. Partial unique indexes ignore the empty placeholder.
ALTER TABLE boards ADD COLUMN slug text NOT NULL DEFAULT '';
ALTER TABLE notes  ADD COLUMN slug text NOT NULL DEFAULT '';

-- Board slugs are globally unique (they stand alone in /board/<slug>); note
-- slugs are unique within their workspace (resolved in the current workspace).
CREATE UNIQUE INDEX boards_slug_key   ON boards (slug)               WHERE slug <> '';
CREATE UNIQUE INDEX notes_ws_slug_key ON notes  (workspace_id, slug) WHERE slug <> '';
