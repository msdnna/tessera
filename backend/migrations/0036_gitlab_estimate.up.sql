-- Phase B+ step 2: bidirectional time-estimate sync. Mirror of due_overridden /
-- start_overridden — a manual estimate edit wins over the GitLab timeEstimate pull.
-- NOTE: gitlab_links is read with SELECT * → apply this together with the new
-- backend (an old running backend would mismatch the column count otherwise).
ALTER TABLE gitlab_links ADD COLUMN estimate_overridden boolean NOT NULL DEFAULT false;
