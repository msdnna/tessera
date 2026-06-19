-- Eisenhower matrix view: an optional manual override of a task's quadrant.
-- NULL = derive the quadrant automatically (importance from priority, urgency from
-- due-date proximity). A non-null value pins the task to a quadrant the user dragged
-- it into (the «сбросить на авто» action clears it back to NULL). Encoding:
--   0 = urgent + important       (срочно/важно)
--   1 = not-urgent + important   (несрочно/важно)
--   2 = urgent + not-important   (срочно/неважно)
--   3 = not-urgent + not-important (несрочно/неважно)
ALTER TABLE tasks ADD COLUMN eisenhower_quadrant smallint;
