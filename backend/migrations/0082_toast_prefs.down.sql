ALTER TABLE notification_prefs DROP COLUMN IF EXISTS dnd_until;
ALTER TABLE notification_prefs DROP COLUMN IF EXISTS quiet_toasts;
ALTER TABLE notification_prefs DROP COLUMN IF EXISTS toasts;
ALTER TABLE notification_prefs DROP COLUMN IF EXISTS toasts_enabled;
