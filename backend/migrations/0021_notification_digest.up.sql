-- Digest / grouping (Phase C): collapse a burst of notifications going to one
-- channel into a single combined message. Additive — safe on the live DB.

-- Per-user digest window. 0 = off (deliver immediately). When > 0, external
-- deliveries are held this many minutes and same-group deliveries are combined.
ALTER TABLE notification_prefs ADD COLUMN digest_minutes integer NOT NULL DEFAULT 0;

-- Grouping key set at enqueue. '' = deliver individually (no digest). Otherwise
-- deliveries sharing a group key (and falling due together) are combined into one
-- message. v1 uses the channel id; the column lets a future per-kind / per-rule
-- grouping change the key without a schema migration.
ALTER TABLE notification_deliveries ADD COLUMN digest_group text NOT NULL DEFAULT '';
