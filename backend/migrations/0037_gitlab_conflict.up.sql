-- GitLab write-back conflict resolution (phase B+): detect when both Tessera and
-- GitLab changed the same field since the last sync, and park the push for an
-- interactive ours/theirs/manual decision instead of silently clobbering. Additive.
--
-- Loose coupling (see CLAUDE.md): all of this lives on the GitLab-owned tables
-- (gitlab_links / gitlab_writebacks); the core task handlers are untouched.

-- gl_snapshot is the last-synced GitLab field state for the linked issue — the
-- baseline ("base") for three-way detection. Written by the pull (from the fetched
-- issue) and after a successful push (refreshLinkSnapshot). Shape (provider-neutral):
--   {title, description, state, due, time_estimate, labels:[], assignee_ids:[],
--    priority_label, milestone_gid}
ALTER TABLE gitlab_links ADD COLUMN gl_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;

-- A push parks here (status='conflict') when GitLab diverged from the baseline AND
-- from our desired value. conflict holds everything the resolver UI needs:
--   {fields: [{field, base, ours, theirs}], detected_at}
-- resolution records the user's choice (ours|theirs|manual) once resolved.
ALTER TABLE gitlab_writebacks ADD COLUMN conflict    jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE gitlab_writebacks ADD COLUMN resolution  text  NOT NULL DEFAULT '';
ALTER TABLE gitlab_writebacks ADD COLUMN resolved_by uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE gitlab_writebacks ADD COLUMN resolved_at timestamptz;

-- At most one open conflict per (task, change_kind): a fresh edit during a conflict
-- refreshes that row's desired value rather than stacking a second one.
CREATE UNIQUE INDEX idx_gitlab_writebacks_conflict
    ON gitlab_writebacks (task_id, change_kind) WHERE status = 'conflict';
