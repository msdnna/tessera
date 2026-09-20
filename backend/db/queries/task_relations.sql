-- name: GetTaskByNumber :one
SELECT t.*
FROM tasks t
JOIN boards b ON b.id = t.board_id
JOIN projects p ON p.id = b.project_id
WHERE p.workspace_id = $1 AND t.number = $2;

-- name: AddTaskRelation :one
INSERT INTO task_relations (task_id, related_task_id, kind, source)
VALUES ($1, $2, $3, 'user')
ON CONFLICT (task_id, related_task_id, kind) DO NOTHING
RETURNING *;

-- ListTaskRelations returns a task's relations as seen FROM that task — both the
-- ones it was linked from and the ones pointing at it.
--
-- A relation is one directed row, so reading `task_id = $1` alone showed the link
-- only to the task that created it: #10 "blocks #20" left #20's «Связи» empty
-- while its journal carried the entry and the Gantt drew the arrow (see
-- ListBoardDependencies, which reads both endpoints). The incoming half is
-- flipped to the reader's point of view — #20 is `blocked_by` #10 — with the same
-- mapping the Go side uses (inverseRelationKind); relates/duplicates are symmetric.
--
-- DISTINCT ON collapses a pair that is on record in both directions into one row
-- per end: the GitLab projection writes the reverse edge deliberately
-- (gitlab_relations.go), and two people linking the same pair from either end
-- produce the same shape. `own` orders the real outgoing row first, so the id the
-- client gets back is this task's own whenever there is one.
--
-- `source` is provider-neutral (user|gitlab): the client shows where a relation came
-- from without this query knowing anything about integrations.
-- name: ListTaskRelations :many
WITH edges AS (
    SELECT r.id, r.related_task_id, r.kind, r.source, r.created_at, 0 AS own
    FROM task_relations r
    WHERE r.task_id = sqlc.arg(task_id)
    UNION ALL
    SELECT r.id, r.task_id AS related_task_id,
           CASE r.kind
               WHEN 'blocks' THEN 'blocked_by'
               WHEN 'blocked_by' THEN 'blocks'
               ELSE r.kind
           END AS kind,
           r.source, r.created_at, 1 AS own
    FROM task_relations r
    WHERE r.related_task_id = sqlc.arg(task_id)
), deduped AS (
    SELECT DISTINCT ON (e.related_task_id, e.kind)
        e.id, e.related_task_id, e.kind, e.source, e.created_at
    FROM edges e
    ORDER BY e.related_task_id, e.kind, e.own, e.created_at
)
SELECT
    d.id, sqlc.arg(task_id)::uuid AS task_id, d.related_task_id, d.kind, d.source, d.created_at,
    t.number AS related_number,
    t.title  AS related_title,
    t.board_id AS related_board_id,
    t.completed_at AS related_completed_at,
    t.archived_at  AS related_archived_at
FROM deduped d
JOIN tasks t ON t.id = d.related_task_id
ORDER BY d.created_at;

-- name: ListBoardDependencies :many
-- Every blocking dependency where BOTH endpoints live on the given board — the
-- Gantt view's whole-board edge graph (drawn as arrows). Rows are returned raw
-- (task_id/related_task_id/kind); the client normalises to blocker→blocked
-- (kind='blocks' → task_id blocks related_task_id; 'blocked_by' → the reverse).
-- The relation id lets the client delete an edge directly.
SELECT r.id, r.task_id, r.related_task_id, r.kind
FROM task_relations r
JOIN tasks t  ON t.id  = r.task_id
JOIN tasks rt ON rt.id = r.related_task_id
WHERE t.board_id = $1 AND rt.board_id = $1
  AND r.kind IN ('blocks', 'blocked_by');

-- RelationExistsBothWays answers "are these two already linked this way?" using
-- the same predicate the ✕ deletes by (DeleteRelationBothWays): a relation is
-- one directed row, but both tasks show it and either end may have made it, so
-- #10 "blocks" #20 and #20 "blocked_by" #10 are the same statement — and the
-- symmetric kinds map to themselves.
--
-- Not the same question as AddTaskRelation's ON CONFLICT, whose key is the row
-- (task_id, related_task_id, kind) and so misses the half written from the other
-- end entirely.
-- name: RelationExistsBothWays :one
SELECT EXISTS (
    SELECT 1 FROM task_relations
    WHERE (task_id = $1 AND related_task_id = $2 AND kind = sqlc.arg(kind))
       OR (task_id = $2 AND related_task_id = $1 AND kind = sqlc.arg(inverse_kind))
);

-- name: GetTaskRelation :one
SELECT * FROM task_relations WHERE id = $1;

-- name: DeleteTaskRelation :exec
DELETE FROM task_relations WHERE id = $1;

-- DeleteRelationsBetween drops EVERY relation between two tasks, in either
-- direction, and reports how many rows went — "/unlink #N" means "these two are
-- no longer linked at all", whatever kinds are on record.
-- name: DeleteRelationsBetween :execrows
DELETE FROM task_relations
WHERE (task_id = $1 AND related_task_id = $2)
   OR (task_id = $2 AND related_task_id = $1);

-- DeleteRelationBothWays drops ONE link between two tasks — the given kind and
-- the row that says the same thing from the other end (blocks ⇄ blocked_by, the
-- symmetric kinds map to themselves). That mirrored row is real: the GitLab
-- projection writes it deliberately, and either user can link the same pair from
-- their own end. Since both ends now SHOW the link (ListTaskRelations reads
-- either direction), deleting only the clicked row would put it straight back.
-- Other kinds between the same two tasks are left alone — "✕" on «связана с»
-- must not also drop «блокирует».
-- name: DeleteRelationBothWays :execrows
DELETE FROM task_relations
WHERE (task_id = $1 AND related_task_id = $2 AND kind = sqlc.arg(kind))
   OR (task_id = $2 AND related_task_id = $1 AND kind = sqlc.arg(inverse_kind));

-- DeleteRelationsAcrossWorkspaces drops every relation of the given project's
-- tasks whose other end is now in a different workspace — the state a project
-- transfer leaves behind. AddRelation resolves its target through
-- GetTaskByNumber scoped to the caller's workspace, so such a pair can only
-- appear when the boundary moves under an existing relation; ListTaskRelations
-- joins tasks for related_number/related_title with no workspace filter of its
-- own, so leaving the row in place hands both sides the other's task titles.
-- Returns the number of relations dropped.
-- name: DeleteRelationsAcrossWorkspaces :execrows
DELETE FROM task_relations r
WHERE (
        r.task_id IN (SELECT t.id FROM tasks t JOIN boards b ON b.id = t.board_id WHERE b.project_id = $1)
     OR r.related_task_id IN (SELECT t.id FROM tasks t JOIN boards b ON b.id = t.board_id WHERE b.project_id = $1)
      )
  AND (SELECT p.workspace_id FROM tasks t JOIN boards b ON b.id = t.board_id
       JOIN projects p ON p.id = b.project_id WHERE t.id = r.task_id)
      IS DISTINCT FROM
      (SELECT p.workspace_id FROM tasks t JOIN boards b ON b.id = t.board_id
       JOIN projects p ON p.id = b.project_id WHERE t.id = r.related_task_id);
