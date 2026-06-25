-- Projects and groups gain an icon_mode: how the chosen colour is applied.
--   'badge' (default, legacy behaviour) — colour fills the icon box, glyph stays white.
--   'icon'                              — box is transparent, the glyph itself is coloured.
ALTER TABLE projects       ADD COLUMN icon_mode text NOT NULL DEFAULT 'badge';
ALTER TABLE project_groups ADD COLUMN icon_mode text NOT NULL DEFAULT 'badge';
