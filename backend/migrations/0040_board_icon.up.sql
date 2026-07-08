-- Boards gain their own icon + colour + icon_mode, mirroring projects/groups, so a
-- board can be branded (edited from the board customize panel). Same semantics as
-- migration 0034: icon_mode 'badge' fills the box, 'icon' tints the glyph.
ALTER TABLE boards ADD COLUMN icon      text NOT NULL DEFAULT '';
ALTER TABLE boards ADD COLUMN color     text NOT NULL DEFAULT '';
ALTER TABLE boards ADD COLUMN icon_mode text NOT NULL DEFAULT 'badge';
