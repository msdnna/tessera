-- Project groups get an icon (curated key or ionicon SVG markup; empty → folder)
-- and a colour (empty/transparent → no background), mirroring projects.
ALTER TABLE project_groups
    ADD COLUMN icon  text NOT NULL DEFAULT '',
    ADD COLUMN color text NOT NULL DEFAULT '';
