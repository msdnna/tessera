ALTER TABLE reminders DROP COLUMN notified_at;
DROP TABLE IF EXISTS due_notification_state;
ALTER TABLE tasks DROP COLUMN due_notify_enabled;
ALTER TABLE tasks DROP COLUMN due_repeat_minutes;
ALTER TABLE tasks DROP COLUMN due_lead_minutes;
DROP TABLE IF EXISTS notification_prefs;
