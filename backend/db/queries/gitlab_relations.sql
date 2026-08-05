-- GitLab linked items (task #2591). Raw remote links live here; the core
-- `task_relations` table only ever sees a provider-neutral source='gitlab'.

-- UpsertGitlabIssueLink records one remote link and stamps it as seen this run.
-- resolved_relation_id is intentionally NOT touched, so RETURNING reports whether
-- the link was already projected onto a core relation (nil ⇒ still to resolve).
-- name: UpsertGitlabIssueLink :one
INSERT INTO gitlab_issue_links (
    integration_id, src_project_path, src_iid, dst_project_path, dst_iid,
    link_type, gl_link_id, gl_web_url, last_seen_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (integration_id, src_project_path, src_iid, dst_project_path, dst_iid, link_type)
DO UPDATE SET gl_link_id = EXCLUDED.gl_link_id, gl_web_url = EXCLUDED.gl_web_url, last_seen_at = now()
RETURNING *;

-- DeleteStaleGitlabIssueLinks drops links GitLab no longer reports, returning the
-- relations they had projected so the caller can unproject them. Scoped to the
-- source iids actually inspected this run — an issue outside the run's scope must
-- keep its links rather than lose them to a "not seen" verdict.
-- name: DeleteStaleGitlabIssueLinks :many
WITH deleted AS (
    DELETE FROM gitlab_issue_links
    WHERE integration_id = $1
      AND src_iid = ANY(sqlc.arg(src_iids)::bigint[])
      AND last_seen_at < sqlc.arg(seen_before)
    RETURNING resolved_relation_id
)
SELECT resolved_relation_id::uuid AS relation_id FROM deleted WHERE resolved_relation_id IS NOT NULL;

-- ListUnresolvedGitlabIssueLinks returns every link with no core relation yet:
-- both the ones just upserted and older ones whose other endpoint had not been
-- imported at the time (deferred resolution).
-- name: ListUnresolvedGitlabIssueLinks :many
SELECT * FROM gitlab_issue_links
WHERE integration_id = $1 AND resolved_relation_id IS NULL
ORDER BY created_at;

-- name: SetGitlabIssueLinkResolved :exec
UPDATE gitlab_issue_links SET resolved_relation_id = $2 WHERE id = $1;

-- GetGitlabLinkByIID resolves a GitLab issue iid to its Tessera task within one
-- binding (gl_iid is unique per integration in practice; LIMIT 1 guards the rest).
-- name: GetGitlabLinkByIID :one
SELECT * FROM gitlab_links WHERE integration_id = $1 AND gl_iid = $2 LIMIT 1;

-- GetGitlabIntegrationByWorkspaceProject finds the binding for a project path
-- inside one workspace — the hop that resolves a cross-project link while keeping
-- relations from ever crossing a workspace boundary.
-- name: GetGitlabIntegrationByWorkspaceProject :one
SELECT * FROM gitlab_integrations WHERE workspace_id = $1 AND project_path = $2 LIMIT 1;

-- AddTaskRelationSourced projects a remote link onto the core table. The conflict
-- branch is a deliberate no-op update: it returns the existing row (so the link can
-- record which relation it maps to) without downgrading a user-made relation's
-- source to 'gitlab'.
-- name: AddTaskRelationSourced :one
INSERT INTO task_relations (task_id, related_task_id, kind, source)
VALUES ($1, $2, $3, $4)
ON CONFLICT (task_id, related_task_id, kind)
DO UPDATE SET source = task_relations.source
RETURNING *;

-- DeleteGitlabRelation unprojects a relation. The source guard means a relation the
-- user has since taken ownership of is never removed by the sync.
-- name: DeleteGitlabRelation :execrows
DELETE FROM task_relations WHERE id = $1 AND source = 'gitlab';
