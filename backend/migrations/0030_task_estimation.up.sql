-- Task estimation. A task carries an optional effort estimate stored in a
-- canonical numeric form (NULL = unestimated). The estimate's *unit* is not on
-- the task — it is resolved from the project's (or, failing that, the
-- workspace's) estimation config, so a single board speaks one unit:
--   • time   → canonical value is MINUTES (ints; "3d 4h" normalises via the
--              working-day/week config below);
--   • points → canonical value is the point number on a scale (1/2/3/5/8/…);
--   • custom → canonical value is the count of a named unit.
ALTER TABLE tasks ADD COLUMN estimate double precision;

-- Two-level estimation config (mirrors tag-prefixes, mig 0026): a workspace-wide
-- default that every project inherits, plus an optional per-project override
-- (e.g. a "Home" project measuring in days while the rest use story points).
-- Both NULL → the built-in default: {"unit":"time","hours_per_day":8,"days_per_week":5}.
-- Shape (provider-neutral, extensible):
--   {"unit":"time","hours_per_day":8,"days_per_week":5}
--   {"unit":"points","points_scale":"fibonacci"|"tshirt"|"linear"}
--   {"unit":"custom","custom_label":"у.е."}
ALTER TABLE workspaces ADD COLUMN estimation jsonb;
ALTER TABLE projects   ADD COLUMN estimation jsonb;
