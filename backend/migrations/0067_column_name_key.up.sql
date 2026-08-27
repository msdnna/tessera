-- Stable keys for the four columns every new board is seeded with (#2800).
--
-- Same shape as 0066 for workspaces: the seed writes both the Russian `name`
-- (kept as the fallback for old clients, GitLab rules and DB lookups) and a
-- language-neutral `name_key`, and the client draws the caption from its own
-- catalogue. A user rename clears the key — a column someone called «Бэклог»
-- must not turn back into "To do" on an English UI.
ALTER TABLE board_columns ADD COLUMN name_key text;

-- Backfill by exact name match against the seeded four. Renamed columns do not
-- match and stay keyless, which is exactly what the rename path enforces from
-- here on.
UPDATE board_columns SET name_key = CASE name
    WHEN 'К работе'        THEN 'todo'
    WHEN 'В процессе'      THEN 'in_progress'
    WHEN 'На рассмотрении' THEN 'review'
    WHEN 'Готово'          THEN 'done'
END
WHERE name IN ('К работе', 'В процессе', 'На рассмотрении', 'Готово');
