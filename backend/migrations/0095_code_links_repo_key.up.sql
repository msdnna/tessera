-- Merge requests of other projects in «Разработка» (#2964 rework).
--
-- A change's number is per-repository (the 12 of !12), and a task now collects
-- changes from several repositories through one provider binding — the issue's
-- MRs often live in other GitLab projects than the one the binding mirrors. So
-- the repository joins the identity of a (task, change) row; before, !12 of two
-- projects would have been one row. Existing rows all carry the binding's own
-- repository, so the new key holds for them as is — safe on the live DB.
ALTER TABLE task_code_links DROP CONSTRAINT task_code_links_pkey;
ALTER TABLE task_code_links ADD PRIMARY KEY (task_id, provider, source_id, kind, repo, number);

DROP INDEX idx_task_code_links_change;
CREATE INDEX idx_task_code_links_change ON task_code_links (provider, source_id, kind, repo, number);
