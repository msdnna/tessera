-- Workspace tiles on the v2 rail (#2955): a colour and an icon per workspace,
-- like projects (0003/0034). Empty icon = the tile shows the name's initials;
-- empty colour = the client derives one from the id.
ALTER TABLE workspaces ADD COLUMN color     text NOT NULL DEFAULT '';
ALTER TABLE workspaces ADD COLUMN icon      text NOT NULL DEFAULT '';
ALTER TABLE workspaces ADD COLUMN icon_mode text NOT NULL DEFAULT 'badge';
