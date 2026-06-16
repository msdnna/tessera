-- Due-date & reminder notifications (Phase B): a background scanner emits
-- notifications ahead of (and repeating up to) a task's due date, and routes
-- reminders to the user's channels at their time. Additive — safe on the live DB.

-- Per-user scheduling defaults. A task may override lead/repeat individually.
CREATE TABLE notification_prefs (
    user_id            uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    due_enabled        boolean NOT NULL DEFAULT true,  -- emit due-date notifications at all
    due_lead_minutes   integer NOT NULL DEFAULT 60,    -- start notifying N minutes before due
    due_repeat_minutes integer NOT NULL DEFAULT 0,     -- repeat every N minutes after that (0 = once)
    reminder_enabled   boolean NOT NULL DEFAULT true,  -- also route reminders to channels at remind_at
    updated_at         timestamptz NOT NULL DEFAULT now()
);

-- Per-task overrides (NULL = inherit the user default). Set from the card's due
-- popover (a reference tracker-style).
ALTER TABLE tasks ADD COLUMN due_lead_minutes   integer;
ALTER TABLE tasks ADD COLUMN due_repeat_minutes integer;
ALTER TABLE tasks ADD COLUMN due_notify_enabled boolean;

-- Per-(task,user) dedup + repeat state. fired_due snapshots the due_date the last
-- fire was for, so editing the due date re-arms the notification without touching
-- the task write paths; last_fired_at drives the repeat interval.
CREATE TABLE due_notification_state (
    task_id       uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    fired_due     timestamptz NOT NULL,
    last_fired_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, user_id)
);

-- Reminders are delivered to channels once at remind_at (alongside the Android
-- local alarm); notified_at marks that the channel delivery has been emitted.
ALTER TABLE reminders ADD COLUMN notified_at timestamptz;
