-- Near-realtime GitLab sync (#2594): accept issue/note webhooks from GitLab in
-- addition to the polling pull. The hook does not map the issue itself — it marks
-- the integration dirty and a debounced worker runs the ordinary incremental pull,
-- so the whole sync engine (labels, columns, subtasks, relations, comments,
-- conflicts, journal) stays a single code path.
--
-- The shared secret is stored encrypted like the PATs (AES-256-GCM via the sealer);
-- an empty value means "no webhook configured" and every delivery is rejected.
-- Additive only — safe to apply to the live database.

ALTER TABLE gitlab_integrations ADD COLUMN webhook_secret_enc text NOT NULL DEFAULT '';
ALTER TABLE gitlab_integrations ADD COLUMN webhook_enabled boolean NOT NULL DEFAULT false;
-- Last accepted delivery, so the UI can tell a live hook from a silent one.
ALTER TABLE gitlab_integrations ADD COLUMN last_webhook_at timestamptz;

-- gitlab_sync_runs.trigger takes the new value 'webhook'; the column is plain text
-- without a CHECK constraint (0033_gitlab_sync_journal), so no change is needed here.
