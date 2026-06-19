-- Task start date: the left edge of a task's bar on the Timeline / Gantt views.
-- A task has only a due_date (the right edge); to draw a start→end segment we need
-- an explicit, optional start. NULL = no start set (the views treat the task as a
-- point at its due date, or unscheduled when both are absent). Additive — safe on
-- the live DB.
ALTER TABLE tasks ADD COLUMN start_date timestamptz;
