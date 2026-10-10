ALTER TABLE conferences DROP COLUMN IF EXISTS soon_notified_for;
ALTER TABLE conference_participants DROP COLUMN IF EXISTS rsvp_at;
ALTER TABLE conference_participants DROP COLUMN IF EXISTS rsvp;
ALTER TABLE notification_prefs DROP COLUMN IF EXISTS quiet_allow_urgent;
