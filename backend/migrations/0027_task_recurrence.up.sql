-- Recurring tasks: a task carries an optional recurrence rule describing how its
-- due date advances when the task is closed (moved into the board's done column or
-- marked completed). NULL = a one-off task. Shape (provider-neutral, extensible):
--   {"freq": "daily"|"weekly"|"monthly"|"yearly", "interval": <int >= 1>}
ALTER TABLE tasks ADD COLUMN recurrence jsonb;
