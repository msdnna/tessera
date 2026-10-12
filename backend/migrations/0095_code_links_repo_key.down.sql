-- Back to one row per (task, change number): of the rows the old key can't tell
-- apart (the same number in two repositories), one is kept.
DELETE FROM task_code_links a
USING task_code_links b
WHERE a.task_id = b.task_id AND a.provider = b.provider AND a.source_id = b.source_id
  AND a.kind = b.kind AND a.number = b.number AND a.ctid < b.ctid;

DROP INDEX idx_task_code_links_change;
CREATE INDEX idx_task_code_links_change ON task_code_links (provider, source_id, kind, number);

ALTER TABLE task_code_links DROP CONSTRAINT task_code_links_pkey;
ALTER TABLE task_code_links ADD PRIMARY KEY (task_id, provider, source_id, kind, number);
