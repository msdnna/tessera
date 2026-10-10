-- Откат 0088: теги и упоминания у записей «Входящих».
ALTER TABLE reminders DROP COLUMN IF EXISTS mentions;
ALTER TABLE reminders DROP COLUMN IF EXISTS tags;
ALTER TABLE notes DROP COLUMN IF EXISTS mentions;
ALTER TABLE notes DROP COLUMN IF EXISTS tags;
