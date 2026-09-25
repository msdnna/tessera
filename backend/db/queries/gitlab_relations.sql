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
--
-- …and to the mirrors of those rows, which the scope alone would never reach
-- (#2932). syncRelations files the reverse edge of a same-project link under the
-- OTHER issue's iid, and an incremental pull asks GitLab only for the issues that
-- changed: deleting a link upstream then usually delivers one of its two ends, so
-- the forward row went while the mirror stayed, and ListTaskRelations — which
-- reads a relation from either direction — kept the deleted link standing on both
-- cards until some later run happened to touch the other issue.
--
-- A mirror is dropped only when it too was missed this run: an end that IS in
-- scope and still reports the link refreshes its own row, and refusing to touch a
-- refreshed row is what keeps this from erasing links GitLab still has. Confined
-- to src_project_path = dst_project_path because that is exactly the case
-- syncRelations mirrors; a cross-project reverse belongs to the other binding's
-- own pruning and must not be pulled into this one's verdict.
-- name: DeleteStaleGitlabIssueLinks :many
WITH stale AS (
    SELECT l.id, l.src_project_path, l.src_iid, l.dst_project_path, l.dst_iid
    FROM gitlab_issue_links l
    WHERE l.integration_id = $1
      AND l.src_iid = ANY(sqlc.arg(src_iids)::bigint[])
      AND l.last_seen_at < sqlc.arg(seen_before)
), mirrors AS (
    SELECT m.id
    FROM gitlab_issue_links m
    JOIN stale s
      ON m.src_project_path = s.dst_project_path AND m.src_iid = s.dst_iid
     AND m.dst_project_path = s.src_project_path AND m.dst_iid = s.src_iid
    WHERE m.integration_id = $1
      AND m.src_project_path = m.dst_project_path
      AND m.last_seen_at < sqlc.arg(seen_before)
), deleted AS (
    DELETE FROM gitlab_issue_links
    WHERE id IN (SELECT id FROM stale UNION SELECT id FROM mirrors)
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
