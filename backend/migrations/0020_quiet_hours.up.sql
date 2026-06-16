-- Quiet hours / silence (Phase B): a per-user window during which external channel
-- deliveries are held (deferred to the window's end) so notifications don't ping at
-- night. In-app notifications are unaffected. Times are minutes-since-midnight in
-- the user's timezone. Additive — safe on the live DB.
ALTER TABLE notification_prefs ADD COLUMN quiet_enabled       boolean NOT NULL DEFAULT false;
ALTER TABLE notification_prefs ADD COLUMN quiet_start_minutes integer NOT NULL DEFAULT 1320; -- 22:00
ALTER TABLE notification_prefs ADD COLUMN quiet_end_minutes   integer NOT NULL DEFAULT 480;  -- 08:00
ALTER TABLE notification_prefs ADD COLUMN quiet_tz            text    NOT NULL DEFAULT '';    -- IANA tz, '' = UTC
