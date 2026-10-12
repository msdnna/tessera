-- name: CreateTask :one
INSERT INTO tasks (
    board_id, column_id, parent_id, title, description, priority, due_date, start_date, estimate, position, created_by, number
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- GetTask answers for live tasks only: a task in the trash (deleted_at, 0086) is
-- gone for every reader, so loadTask and every internal lookup see a 404 / no
-- row. GetTaskAny is the door for the few callers that must see the trash too —
-- restore, purge, and the GitLab sync, which keeps the link of a deleted mirror
-- and must not mistake it for an issue it has never seen.
-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1 AND deleted_at IS NULL;

-- name: GetTaskAny :one
SELECT * FROM tasks WHERE id = $1;

-- name: ListTasksByBoard :many
SELECT * FROM tasks WHERE board_id = $1 AND parent_id IS NULL AND deleted_at IS NULL ORDER BY position, created_at, id;

-- ListBoardTasksWithMeta returns top-level board tasks with their tag and
-- assignee ids aggregated, so the kanban can render chips and group by tag
-- without an extra round-trip per card.
--
-- The three M:N sets are gathered in LATERAL subqueries rather than joined. Joining
-- them produced a row per *combination* — a task with 4 tags, 3 assignees and 2 GitLab
-- assignees expanded to 24 rows, 23 of which array_agg(DISTINCT …) threw away — and
-- this is the board's hot query. Each LATERAL is an index scan on the table's
-- (task_id, …) primary key, and the output is one row per task. gitlab_links stays a
-- plain LEFT JOIN: task_id is its primary key, so it is 1:1 and never multiplied rows.
-- DISTINCT is kept inside the aggregates so the arrays are ordered and deduplicated
-- exactly as before (gl_name and gl_username are each sorted on their own value).
-- name: ListBoardTasksWithMeta :many
SELECT
    t.*,
    tg.tag_ids,
    asg.assignee_ids,
    ga.gitlab_assignees,
    ga.gitlab_assignee_logins,
    gl.gl_iid AS gitlab_iid,
    gl.gl_web_url AS gitlab_url,
    gl.gl_author AS gitlab_author,
    gl.gl_author_name AS gitlab_author_name,
    gl.gl_author_avatar_url AS gitlab_author_avatar_url
FROM tasks t
LEFT JOIN LATERAL (
    SELECT COALESCE(array_agg(DISTINCT tt.tag_id), '{}')::uuid[] AS tag_ids
    FROM task_tags tt WHERE tt.task_id = t.id
) tg ON true
LEFT JOIN LATERAL (
    SELECT COALESCE(array_agg(DISTINCT ta.user_id), '{}')::uuid[] AS assignee_ids
    FROM task_assignees ta WHERE ta.task_id = t.id
) asg ON true
LEFT JOIN LATERAL (
    SELECT
        COALESCE(array_agg(DISTINCT tga.gl_name), '{}')::text[] AS gitlab_assignees,
        COALESCE(array_agg(DISTINCT tga.gl_username), '{}')::text[] AS gitlab_assignee_logins
    FROM task_gitlab_assignees tga WHERE tga.task_id = t.id
) ga ON true
LEFT JOIN gitlab_links gl ON gl.task_id = t.id
WHERE t.board_id = @board_id
  -- The trash (0086) is a screen of its own (ListBoardTrashWithMeta).
  AND t.deleted_at IS NULL
  -- Active board (archived_at IS NULL) or the read-only archive view (IS NOT NULL).
  AND ((@archived::boolean AND t.archived_at IS NOT NULL) OR (NOT @archived::boolean AND t.archived_at IS NULL))
  -- Top-level *for this view*: no parent that the same view would also draw.
  -- `parent_id IS NULL` alone was right only while parent and child always shared
  -- an archived state, which the manual archive guarantees (ArchiveTaskCascade
  -- takes the subtree, ?subtasks=detach re-parents it to the root) and the GitLab
  -- pull does not: applyClosedPolicy archives per issue, so a parent issue closed
  -- inside a closed milestone goes to the archive while a child issue that is
  -- still open stays active. Such a child was fetched by neither screen — the
  -- board hangs subtasks off a parent card in the same payload and the parent is
  -- archived, the archive filters on archived_at IS NOT NULL and the child's is
  -- NULL. It is a card here instead, which is also what the archive view has
  -- always done with a child buried under an archived root (ListBoardArchivedWithMeta).
  AND NOT EXISTS (
    SELECT 1 FROM tasks p
    WHERE p.id = t.parent_id
      AND p.deleted_at IS NULL
      AND ((@archived::boolean AND p.archived_at IS NOT NULL) OR (NOT @archived::boolean AND p.archived_at IS NULL))
  )
  AND (
    (NOT @backlog::boolean AND sqlc.narg('milestone_id')::uuid IS NULL)             -- all (no scope)
    OR (@backlog::boolean AND t.milestone_id IS NULL)                               -- backlog (no milestone)
    OR (sqlc.narg('milestone_id')::uuid IS NOT NULL AND t.milestone_id = sqlc.narg('milestone_id')) -- one milestone
  )
-- Newest-archived first in the archive; board position otherwise. created_at/id
-- break position ties: two cards can share a position (legacy rows from before
-- the midpoint rebalance), and ORDER BY position alone leaves their order to the
-- planner, so the board reshuffles them between reads.
ORDER BY CASE WHEN @archived::boolean THEN t.archived_at END DESC NULLS LAST, t.position, t.created_at, t.id;

-- name: ListSubtasks :many
SELECT * FROM tasks WHERE parent_id = $1 AND deleted_at IS NULL ORDER BY position, created_at, id;

-- ListSubtasksWithMeta returns a parent's subtasks with tag/assignee ids, so the
-- task modal can render a kanban-style hover card for each one.
--
-- The three gl_* columns are the subtask's GitLab hierarchy state (#2592), which the
-- subtasks tab renders per row: gl_iid non-null means the subtask has its own issue,
-- and an EMPTY gl_parent_global_id on top of that is precisely "created and linked,
-- but GitLab did not accept it into the hierarchy" — the state that offers a retry.
-- Joined here rather than fetched per row because the tab renders the whole list at
-- once; gitlab_links is keyed by task_id, so this cannot multiply rows.
-- name: ListSubtasksWithMeta :many
SELECT
    t.*,
    COALESCE(array_agg(DISTINCT tt.tag_id) FILTER (WHERE tt.tag_id IS NOT NULL), '{}')::uuid[] AS tag_ids,
    COALESCE(array_agg(DISTINCT ta.user_id) FILTER (WHERE ta.user_id IS NOT NULL), '{}')::uuid[] AS assignee_ids,
    gl.gl_iid,
    gl.gl_web_url,
    gl.gl_parent_global_id
FROM tasks t
LEFT JOIN task_tags tt ON tt.task_id = t.id
LEFT JOIN task_assignees ta ON ta.task_id = t.id
LEFT JOIN gitlab_links gl ON gl.task_id = t.id
WHERE t.parent_id = $1 AND t.deleted_at IS NULL
GROUP BY t.id, gl.task_id
ORDER BY t.position, t.created_at, t.id;

-- ListBoardSubtasksWithMeta returns every subtask on a board (parent_id set)
-- with tag/assignee ids, so the kanban can render them under their parents.
-- gitlab_assignee_logins mirrors the top-level query so the composer's
-- "gl:<login>" assignee filter matches subtasks too.
--
-- LATERAL for the same reason as ListBoardTasksWithMeta: this fires on the same
-- board render, so joining the three M:N sets multiplied rows here too.
-- name: ListBoardSubtasksWithMeta :many
SELECT
    t.*,
    tg.tag_ids,
    asg.assignee_ids,
    ga.gitlab_assignee_logins
FROM tasks t
LEFT JOIN LATERAL (
    SELECT COALESCE(array_agg(DISTINCT tt.tag_id), '{}')::uuid[] AS tag_ids
    FROM task_tags tt WHERE tt.task_id = t.id
) tg ON true
LEFT JOIN LATERAL (
    SELECT COALESCE(array_agg(DISTINCT ta.user_id), '{}')::uuid[] AS assignee_ids
    FROM task_assignees ta WHERE ta.task_id = t.id
) asg ON true
LEFT JOIN LATERAL (
    SELECT COALESCE(array_agg(DISTINCT tga.gl_username), '{}')::text[] AS gitlab_assignee_logins
    FROM task_gitlab_assignees tga WHERE tga.task_id = t.id
) ga ON true
WHERE t.board_id = $1 AND t.archived_at IS NULL AND t.deleted_at IS NULL
  -- Only rows the board can actually place under a card: the parent has to be
  -- active too, or the subtask is drawn nowhere. The mirror of the predicate in
  -- ListBoardTasksWithMeta, and the reason the two never overlap — an active
  -- child of an archived parent is served there as a card, exactly once.
  AND EXISTS (SELECT 1 FROM tasks p WHERE p.id = t.parent_id AND p.archived_at IS NULL AND p.deleted_at IS NULL)
ORDER BY t.position, t.created_at, t.id;

-- ListBoardArchivedWithMeta returns a board's archived tasks: top-level ones
-- and individually-archived subtasks (whose parent is still active). Children
-- archived together with their parent are hidden (they restore with it).
-- name: ListBoardArchivedWithMeta :many
SELECT
    t.*,
    COALESCE(array_agg(DISTINCT tt.tag_id) FILTER (WHERE tt.tag_id IS NOT NULL), '{}')::uuid[] AS tag_ids,
    COALESCE(array_agg(DISTINCT ta.user_id) FILTER (WHERE ta.user_id IS NOT NULL), '{}')::uuid[] AS assignee_ids
FROM tasks t
LEFT JOIN task_tags tt ON tt.task_id = t.id
LEFT JOIN task_assignees ta ON ta.task_id = t.id
WHERE t.board_id = $1
  AND t.archived_at IS NOT NULL
  AND t.deleted_at IS NULL
  AND (
    t.parent_id IS NULL
    OR NOT EXISTS (SELECT 1 FROM tasks p WHERE p.id = t.parent_id AND p.archived_at IS NOT NULL AND p.deleted_at IS NULL)
  )
GROUP BY t.id
ORDER BY t.archived_at DESC;

-- ListBoardTrashWithMeta is the board's trash (0086): every task that went there
-- as an entry of its own — a root deleted with its subtree, or a subtask deleted
-- on its own. Subtasks that went with their root (same deleted_at stamp, see
-- SoftDeleteTaskSubtree) are not listed: they come back with it. Newest first;
-- deleted_by_name says who threw it away.
-- name: ListBoardTrashWithMeta :many
SELECT
    t.*,
    COALESCE(u.name, '')::text AS deleted_by_name,
    COALESCE((SELECT array_agg(DISTINCT tt.tag_id) FROM task_tags tt WHERE tt.task_id = t.id), '{}')::uuid[] AS tag_ids,
    COALESCE((SELECT array_agg(DISTINCT ta.user_id) FROM task_assignees ta WHERE ta.task_id = t.id), '{}')::uuid[] AS assignee_ids,
    (SELECT count(*) FROM tasks s WHERE s.parent_id = t.id AND s.deleted_at = t.deleted_at)::bigint AS subtask_count
FROM tasks t
LEFT JOIN users u ON u.id = t.deleted_by
WHERE t.board_id = $1
  AND t.deleted_at IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM tasks p WHERE p.id = t.parent_id AND p.deleted_at = t.deleted_at
  )
ORDER BY t.deleted_at DESC, t.number DESC;

-- SoftDeleteTaskSubtree puts a task and every live descendant in the trash with
-- ONE stamp (now() is the transaction timestamp) — what RestoreTaskSubtree reads
-- to give back exactly this delete. A subtask already in the trash keeps the
-- older stamp it went in with: it is an entry of its own there. Returns the ids
-- it took (the realtime event and the uploads purge later need no walk).
-- name: SoftDeleteTaskSubtree :many
WITH RECURSIVE down AS (
    SELECT t.id FROM tasks t WHERE t.id = @id AND t.deleted_at IS NULL
    UNION
    SELECT t.id FROM tasks t JOIN down ON t.parent_id = down.id WHERE t.deleted_at IS NULL
)
UPDATE tasks SET deleted_at = now(), deleted_by = sqlc.narg('deleted_by')::uuid, updated_at = now()
WHERE id IN (SELECT id FROM down)
RETURNING id;

-- RestoreTaskSubtree takes a task out of the trash with the subtasks that went
-- in with it (same stamp — the RestoreTask trick above). A root whose parent is
-- still in the trash comes back to the top level of its column: the parent is
-- gone for every reader, and a subtask under it would be drawn nowhere. The
-- caller passes that decision as detach (and the end-of-column position).
-- name: RestoreTaskSubtree :many
WITH RECURSIVE down AS (
    SELECT t.id, t.deleted_at, t.deleted_at AS root_deleted_at
    FROM tasks t WHERE t.id = @id AND t.deleted_at IS NOT NULL
    UNION
    SELECT t.id, t.deleted_at, down.root_deleted_at
    FROM tasks t JOIN down ON t.parent_id = down.id
), taken AS (
    SELECT down.id FROM down WHERE down.deleted_at = down.root_deleted_at
)
UPDATE tasks SET deleted_at = NULL, deleted_by = NULL, updated_at = now(),
    parent_id = CASE WHEN tasks.id = @id AND @detach::boolean THEN NULL ELSE tasks.parent_id END,
    position = CASE WHEN tasks.id = @id AND @detach::boolean THEN @position::double precision ELSE tasks.position END
WHERE tasks.id IN (SELECT taken.id FROM taken)
RETURNING tasks.id;

-- PurgeDeletedTasks finishes what the trash left pending: tasks that stayed
-- deleted past the cutoff are removed for good (subtasks by the parent_id
-- cascade; the recursion names them so their upload folders can be swept).
-- name: PurgeDeletedTasks :many
WITH RECURSIVE down AS (
    SELECT t.id FROM tasks t WHERE t.deleted_at IS NOT NULL AND t.deleted_at < @cutoff::timestamptz
    UNION
    SELECT t.id FROM tasks t JOIN down ON t.parent_id = down.id
)
DELETE FROM tasks WHERE tasks.id IN (SELECT id FROM down) RETURNING tasks.id;

-- name: ArchiveTask :exec
UPDATE tasks SET archived_at = now(), updated_at = now() WHERE id = $1;

-- ArchiveTaskCascade/RestoreTask walk the whole subtree, not `parent_id = $1`:
-- nesting is not depth-limited (same reason MoveSubtreeToBoard above is
-- recursive), so a one-level archive left grandchildren active under an
-- archived parent — and such a row is drawn by neither screen. The board draws a
-- subtask only under a parent card in the same payload, and that parent is
-- archived; the archive list filters on archived_at IS NOT NULL, which the
-- grandchild's is not. Restore has to match, or the tree comes back partial.
--
-- UNION rather than UNION ALL for the same reason as TaskAncestorIDs: an install
-- that already grew a ring must get an answer instead of spinning.
-- name: ArchiveTaskCascade :exec
WITH RECURSIVE down AS (
    SELECT t.id FROM tasks t WHERE t.id = $1 AND t.deleted_at IS NULL
    UNION
    SELECT t.id FROM tasks t JOIN down ON t.parent_id = down.id WHERE t.deleted_at IS NULL
)
UPDATE tasks SET archived_at = now(), updated_at = now()
-- Only the rows this archive actually takes. A subtask already in the archive
-- keeps the stamp it went in with: that is the date the archive list sorts on
-- (archived_at DESC), so re-stamping reshuffled entries nobody touched — and it
-- erased the one thing that tells "went in with this root" from "was already
-- there", which is what RestoreTask below reads to know what to give back.
WHERE id IN (SELECT id FROM down) AND archived_at IS NULL;

-- RestoreTask gives back what THIS archive took, which is not the same as the
-- whole subtree: a subtask can go to the archive on its own, long before its
-- parent does. The archive view lists such a row as an entry of its own
-- (ListBoardArchivedWithMeta below, the `parent_id IS NULL OR parent is active`
-- branch), so it is a deliberate gesture with its own undo. Restoring the whole
-- subtree undid it too — the subtask came back onto the board together with the
-- ones the cascade had taken, with nothing said anywhere, and the archive the
-- user would go looking in no longer held it.
--
-- "Archived with this root" is readable from the data: ArchiveTaskCascade stamps
-- the subtree in a single UPDATE, so every row it took carries the root's exact
-- archived_at (now() is the transaction timestamp). A subtask archived in an
-- earlier request carries an older one and stays put. The root's stamp rides
-- down the walk rather than being re-read in a second subquery — sqlc cannot
-- resolve a CTE alias next to a second `tasks` reference in an UPDATE's WHERE.
-- The root matches its own stamp, so it is always in; a root that is not
-- archived at all matches nothing (NULL = NULL is unknown), which is the no-op
-- restoring an active task already was.
-- name: RestoreTask :exec
WITH RECURSIVE down AS (
    SELECT t.id, t.archived_at, t.archived_at AS root_archived_at
    FROM tasks t WHERE t.id = $1 AND t.deleted_at IS NULL
    UNION
    SELECT t.id, t.archived_at, down.root_archived_at
    FROM tasks t JOIN down ON t.parent_id = down.id WHERE t.deleted_at IS NULL
), taken AS (
    SELECT down.id FROM down WHERE down.archived_at = down.root_archived_at
)
UPDATE tasks SET archived_at = NULL, updated_at = now()
WHERE id IN (SELECT id FROM taken);

-- ArchiveTaskIfActive/RestoreTaskIfArchived are idempotent single-task variants for
-- the GitLab sync (closed_policy=archive_closed_sprints): no-op + no updated_at churn
-- when the task is already in the target state.
-- name: ArchiveTaskIfActive :exec
UPDATE tasks SET archived_at = now(), updated_at = now() WHERE id = $1 AND archived_at IS NULL;

-- name: RestoreTaskIfArchived :exec
UPDATE tasks SET archived_at = NULL, updated_at = now() WHERE id = $1 AND archived_at IS NOT NULL;

-- TransferTask drops the milestone when the target board belongs to another
-- project. A milestone hangs off a project — it is created under one, and the
-- task modal offers only the ones its own project owns — but ListWorkspaceMilestones
-- rolls tasks up over `t.milestone_id = m.id` with no project filter of its own.
-- So a task that left for another project kept inflating the total / done / Σ
-- estimate of a milestone whose board no longer holds it, while the modal showed
-- «Не задан» (the id isn't in the picker's list) — a number nobody could explain
-- pinned to a link nobody could see. Same resolution as
-- ClearMilestonesAcrossWorkspaces uses for the cross-workspace case: unpin.
-- name: TransferTask :one
UPDATE tasks
SET board_id = $2, column_id = $3, parent_id = NULL, position = $4,
    milestone_id = CASE
        WHEN (SELECT m.project_id FROM milestones m WHERE m.id = tasks.milestone_id)
             IS NOT DISTINCT FROM
             (SELECT b.project_id FROM boards b WHERE b.id = $2)
        THEN tasks.milestone_id ELSE NULL END,
    updated_at = now()
WHERE tasks.id = $1
RETURNING *;

-- MoveSubtreeToBoard puts every descendant of a task on the given board and
-- column. The whole tree, not `parent_id = $1`: nesting is not depth-limited
-- (CreateTask hangs a subtask off a subtask, and TaskAncestorIDs below exists
-- because chains run deeper than one), so a one-level move left grandchildren
-- on the board their parent had just left. A subtask is drawn only under a
-- parent card on the same board, so such a row is drawn on neither board — and
-- board_id/column_id are ON DELETE CASCADE, so deleting that stale column takes
-- the task with it.
--
-- UNION rather than UNION ALL for the same reason as TaskAncestorIDs: an install
-- that already grew a ring must get an answer instead of spinning.
-- name: MoveSubtreeToBoard :exec
WITH RECURSIVE down AS (
    SELECT t.id FROM tasks t WHERE t.parent_id = $1
    UNION
    SELECT t.id FROM tasks t JOIN down ON t.parent_id = down.id
)
UPDATE tasks SET board_id = $2, column_id = $3,
    milestone_id = CASE
        WHEN (SELECT m.project_id FROM milestones m WHERE m.id = tasks.milestone_id)
             IS NOT DISTINCT FROM
             (SELECT b.project_id FROM boards b WHERE b.id = $2)
        THEN tasks.milestone_id ELSE NULL END,
    updated_at = now()
WHERE tasks.id IN (SELECT down.id FROM down);

-- ReopenSubtreeLeavingDone reopens the tasks of a subtree (the root included)
-- that stand in @done_column_id and are marked completed. Run BEFORE the two
-- statements above, inside their transaction: it reads the column a task is
-- leaving, which TransferTask/MoveSubtreeToBoard are about to overwrite.
--
-- Completion follows the column — that is the board's rule (applyMove: into the
-- done column completes, out of it reopens). A transfer changes the column too,
-- but it goes nowhere near applyMove, so a finished card carried to another
-- board landed in that board's first column still marked done: struck through in
-- «К работе», counted as completed, and out of reach of the reopen rule, which
-- only ever fires for a task standing in a done column. The condition is the
-- source column, not the flag alone: a task completed anywhere else keeps its
-- flag, exactly as an in-board move would leave it — a board without a done
-- column does not tie completion to columns at all, and transferring must not
-- invent that tie.
-- name: ReopenSubtreeLeavingDone :many
WITH RECURSIVE down AS (
    SELECT t.id FROM tasks t WHERE t.id = @root_id
    UNION
    SELECT t.id FROM tasks t JOIN down ON t.parent_id = down.id
), stuck AS (
    -- The walk carries ids only and the condition is read back off `tasks`: sqlc
    -- resolves a recursive CTE's own columns nowhere but its select list (same
    -- corner TaskAncestorIDs notes), and `down.column_id` in a WHERE fails
    -- generation with «table alias "down" does not exist».
    SELECT t.id FROM tasks t
    WHERE t.id IN (SELECT down.id FROM down)
      AND t.column_id = @done_column_id
      AND t.completed_at IS NOT NULL
)
UPDATE tasks SET completed_at = NULL, updated_at = now()
WHERE tasks.id IN (SELECT stuck.id FROM stuck)
RETURNING tasks.id;

-- ReopenTaskLeavingDone is the statement above for ONE card — the same
-- condition (standing in @done_column_id and marked completed), without the
-- walk. A re-parent inside one board moves only the task itself: its
-- descendants keep the column each of them sits in (applyParent re-boards a
-- subtree only when the board changes), so reopening the whole subtree there
-- would clear the flag of a finished child that never left «Готово».
-- name: ReopenTaskLeavingDone :many
UPDATE tasks SET completed_at = NULL, updated_at = now()
WHERE tasks.id = @root_id
  AND tasks.column_id = @done_column_id
  AND tasks.completed_at IS NOT NULL
RETURNING tasks.id;

-- TaskAncestorIDs returns the task itself followed by its parents, up to the
-- root. Used to reject attaching a task below one of its own descendants, which
-- would leave the branch ringed with no root — and a subtask nobody can reach
-- from a root is drawn nowhere, so the whole ring drops off the board. Nesting
-- is not depth-limited, so the one-level check this replaced was not enough.
-- Mirrors ProjectGroupAncestorIDs and DocumentAncestorIDs, membership test in Go
-- included (sqlc cannot resolve a recursive CTE inside a subquery).
--
-- UNION rather than UNION ALL: an install that already grew a ring through the
-- unguarded handler must still get an answer here instead of spinning, and
-- dedup on the row is what stops the walk when it comes back around.
-- name: TaskAncestorIDs :many
WITH RECURSIVE up AS (
    SELECT a.id, a.parent_id FROM tasks a WHERE a.id = $1
    UNION
    SELECT t.id, t.parent_id FROM tasks t JOIN up ON t.id = up.parent_id
)
SELECT id FROM up;

-- SetTaskParent is the other door onto board_id (a task adopts its new parent's
-- board), so it unpins a milestone left behind in another project for the same
-- reason TransferTask does.
--
-- It also writes `position`, because changing the parent changes which LIST the
-- task is ordered in: a subtask's float8 ladder runs per PARENT, a card's per
-- COLUMN, and both start at the same positionGap. Leaving the old number in
-- place put the first subtask of a parent (65536) on the exact position of the
-- column's first card — the dead slot positionExhausted describes — and a later
-- sibling (196608) somewhere in the middle of the column. The caller computes
-- the end of the destination list; the board's drag already sent a move right
-- after this call, and now the other doors (the modal's «Открепить», a card
-- dropped into a card's subtask list) land somewhere predictable too.
-- name: SetTaskParent :one
UPDATE tasks
SET parent_id = $2, board_id = $3, column_id = $4, position = $5,
    milestone_id = CASE
        WHEN (SELECT m.project_id FROM milestones m WHERE m.id = tasks.milestone_id)
             IS NOT DISTINCT FROM
             (SELECT b.project_id FROM boards b WHERE b.id = $3)
        THEN tasks.milestone_id ELSE NULL END,
    updated_at = now()
WHERE tasks.id = $1
RETURNING *;

-- DetachChildren empties a parent's subtask list onto the board, and appends the
-- orphans to the end of the top-level ladder of whatever column each of them
-- stands in — for the same reason SetTaskParent writes a position. Keeping their
-- old numbers dropped the whole brood into the middle of the column, the first
-- child landing exactly on the first card.
--
-- The `max` subquery is correlated to the row's own column (a subtask may stand
-- in another column than its parent) and reads the pre-UPDATE snapshot, where
-- the children being detached still hang off the parent — so it sees the cards
-- that are staying and nothing else. `rn` spaces the brood out by one gap each,
-- keeping the order the subtask list had.
--
-- Written as UPDATE … FROM (subquery) rather than a CTE: sqlc mis-parses
-- UPDATE … FROM with a leading WITH.
--
-- RETURNING the ids because the caller owes GitLab a child_detach per child it
-- just dropped to top-level (#2932) — the same push PATCH /tasks/:id/parent
-- sends for the same gesture. They have to be named: nothing else does, the
-- parent is archived or gone by then, and a walk after the fact finds no
-- children at all.
-- name: DetachChildren :many
UPDATE tasks t
SET parent_id = NULL,
    position = (
        SELECT coalesce(max(p.position), 0) FROM tasks p
        WHERE p.column_id = t.column_id AND p.parent_id IS NULL AND p.deleted_at IS NULL
    ) + 65536 * ord.rn,
    updated_at = now()
FROM (
    SELECT s.id, row_number() OVER (ORDER BY s.position, s.created_at, s.id) AS rn
    FROM tasks s WHERE s.parent_id = $1 AND s.deleted_at IS NULL
) ord
WHERE t.id = ord.id
RETURNING t.id;

-- name: MaxTaskPositionInColumn :one
SELECT coalesce(max(position), 0)::double precision
FROM tasks WHERE column_id = $1 AND parent_id IS NULL AND deleted_at IS NULL;

-- Rebalance* respace a sibling list onto a fresh gap * 1,2,3… ladder, keeping the
-- order the list already had. Card order is a float8 midpoint, and the gap halves
-- on every drop into the same slot, so after ~52 drops the average of two
-- neighbours *is* one of them; the reorder handler calls these when it sees that
-- and then recomputes, which is the only way the slot ever opens up again.
-- (Written as UPDATE … FROM (subquery) rather than a CTE: sqlc mis-parses
-- UPDATE … FROM with a leading WITH.)
-- name: RebalanceColumnTaskPositions :exec
UPDATE tasks t
SET position = ordered.rn * sqlc.arg(gap)::double precision, updated_at = now()
FROM (
    SELECT id, row_number() OVER (ORDER BY position, created_at, id) AS rn
    FROM tasks WHERE column_id = sqlc.arg(column_id)::uuid AND parent_id IS NULL AND deleted_at IS NULL
) ordered
WHERE t.id = ordered.id;

-- name: RebalanceSubtaskPositions :exec
UPDATE tasks t
SET position = ordered.rn * sqlc.arg(gap)::double precision, updated_at = now()
FROM (
    SELECT id, row_number() OVER (ORDER BY position, created_at, id) AS rn
    FROM tasks WHERE parent_id = sqlc.arg(parent_id)::uuid AND deleted_at IS NULL
) ordered
WHERE t.id = ordered.id;

-- UpdateTask writes a task's editable fields, optionally under an optimistic
-- lock: pass expected_updated_at with the updated_at the client read, and the
-- write only lands if nobody touched the row since — no row comes back
-- otherwise, so check and write stay in one statement with no gap to race in.
-- NULL (the default) means no check, which is how every internal caller writes.
-- The comparison is truncated to milliseconds because clients round-trip the
-- timestamp through JS Date, which drops the microseconds Postgres stores.
-- name: UpdateTask :one
UPDATE tasks
SET title = $2, description = $3, priority = $4, due_date = $5, completed_at = $6,
    recurrence = $7, start_date = $8, estimate = $9, updated_at = now()
WHERE id = $1
  AND (sqlc.narg('expected_updated_at')::timestamptz IS NULL
       OR date_trunc('milliseconds', updated_at)
          = date_trunc('milliseconds', sqlc.narg('expected_updated_at')::timestamptz))
RETURNING *;

-- ReopenSubtasks clears the completion of a recurring task's direct subtasks when
-- the parent recurs, so a checklist starts fresh for the next occurrence.
--
-- An archived item is not part of the routine any more, so all three statements
-- here spare it (the reason cloneChecklist gives for skipping it in the duplicate:
-- archiving one item while keeping the rest is a deliberate gesture with its own
-- undo). archived_at itself was never touched, so the board showed nothing — but
-- the archive entry lost its tick, left «Готово» for the rule's target column and
-- took a period onto its dates, every occurrence, and handed that back on restore.
-- name: ReopenSubtasks :exec
UPDATE tasks SET completed_at = NULL, updated_at = now()
WHERE parent_id = $1 AND completed_at IS NOT NULL AND archived_at IS NULL;

-- LiftSubtasksFromColumn takes a recurring parent's direct subtasks out of the
-- board's done column when the parent recurs. On this board "in the done column"
-- and "completed" are one fact, kept in step by MoveTask's auto-toggle: clearing
-- completed_at alone leaves an item the board still draws under «Готово» with an
-- empty checkbox, and the next move out of that column then finds nothing to
-- reopen. Position is deliberately untouched — it is what orders the checklist
-- among its siblings, and the recurrence is not a reordering.
-- name: LiftSubtasksFromColumn :exec
UPDATE tasks SET column_id = sqlc.arg(target_column)::uuid, updated_at = now()
WHERE parent_id = sqlc.arg(parent_id)::uuid
  AND column_id = sqlc.arg(from_column)::uuid
  AND archived_at IS NULL;

-- ShiftSubtaskWindow moves a recurring parent's direct subtasks by the same step
-- its due date just took, so the whole checklist travels to the next occurrence
-- instead of staying pinned — and overdue — at the first one. What has to survive
-- is the length of each item's window (the reason shiftStart gives), so the step
-- is applied to the *clock*, not to the instant: the shift is read on the rule's
-- calendar (recur.Rule.WallStep) and added inside the rule's zone, which is the
-- calendar recur.Next stepped the parent's due date on. Added to the instant
-- instead, the 23-hour day a zone loses to a spring-forward moved every item that
-- did not itself straddle the transition an hour off its parent — and one
-- scheduled just after midnight onto the previous date (#2932). NULL + interval
-- is NULL, so an item nobody scheduled keeps no dates.
-- name: ShiftSubtaskWindow :exec
UPDATE tasks
SET due_date   = (due_date   AT TIME ZONE sqlc.arg(zone)::text
                    + make_interval(secs => sqlc.arg(shift_seconds)::double precision))
                 AT TIME ZONE sqlc.arg(zone)::text,
    start_date = (start_date AT TIME ZONE sqlc.arg(zone)::text
                    + make_interval(secs => sqlc.arg(shift_seconds)::double precision))
                 AT TIME ZONE sqlc.arg(zone)::text,
    updated_at = now()
WHERE parent_id = sqlc.arg(parent_id)::uuid
  AND (due_date IS NOT NULL OR start_date IS NOT NULL)
  AND archived_at IS NULL;

-- ListScheduleRecurDue returns tasks whose recurrence fires on a schedule and
-- whose due date has passed — the schedule worker advances each one.
--
-- A completed task is deliberately still in: this trigger is the clock-driven one
-- ("due date passes, no close needed"), so closing an occurrence has to be an
-- ordinary act — the occurrence is done, the series is not. Filtering on
-- completed_at IS NULL, the way every other sweep here does, made one tick of the
-- checkbox (or one drag into the done column, which completes the card the same
-- way) end the recurrence for good: nothing else picks the task up, because
-- recurAdvance runs on completion with the "complete" trigger and a schedule rule
-- doesn't match it. The card stayed closed with its rule still on it, and the
-- editor kept promising a repeat that was never coming. applyRecur's reschedule
-- branch already clears completed_at and moves the task to the rule's target
-- column, which is exactly the next occurrence arriving.
--
-- Archived tasks stay out: the archive is the one place where a card is meant to
-- stop moving, and a series resurrecting itself out of it would be a card nobody
-- can see arriving.
-- name: ListScheduleRecurDue :many
SELECT * FROM tasks
WHERE archived_at IS NULL
  AND deleted_at IS NULL
  AND due_date IS NOT NULL
  AND due_date <= now()
  AND recurrence->>'trigger' = 'schedule'
  -- An archived project (0086) is read-only: its series wait for the restore.
  AND EXISTS (SELECT 1 FROM boards b JOIN projects p ON p.id = b.project_id
              WHERE b.id = tasks.board_id AND b.deleted_at IS NULL AND p.archived_at IS NULL)
ORDER BY due_date;

-- name: UpdateTaskDueDate :exec
UPDATE tasks SET due_date = $2, updated_at = now() WHERE id = $1;

-- name: UpdateTaskStartDate :exec
UPDATE tasks SET start_date = $2, updated_at = now() WHERE id = $1;

-- name: UpdateTaskEstimate :exec
UPDATE tasks SET estimate = $2, updated_at = now() WHERE id = $1;

-- SetTaskTitle / SetTaskDescription apply a single resolved field from a write-back
-- conflict resolution (theirs/manual) without disturbing the other columns.
-- name: SetTaskTitle :exec
UPDATE tasks SET title = $2, updated_at = now() WHERE id = $1;

-- name: SetTaskDescription :exec
UPDATE tasks SET description = $2, updated_at = now() WHERE id = $1;

-- name: SetTaskPriority :exec
UPDATE tasks SET priority = $2, updated_at = now() WHERE id = $1;

-- SetTaskColumnCompleted applies a resolved state conflict: move the task to a
-- column and set/clear its completion (used when accepting GitLab's open/closed).
-- name: SetTaskColumnCompleted :exec
UPDATE tasks SET column_id = $2, completed_at = $3, updated_at = now() WHERE id = $1;

-- name: MoveTask :one
UPDATE tasks
SET column_id = $2, position = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: TaskSubtreeParticipants :many
-- TaskSubtreeParticipants names who has to hear that a task is being deleted:
-- the assignees and the creator of the task and of every subtask the delete
-- takes with it by cascade. Read before the delete — afterwards the rows are gone
-- and the notification has to carry the task's number and title itself.
--
-- One row per person, about the task nearest the root they took part in: a
-- participant of the root hears about the root (its subtasks are implied), a
-- participant of a subtask alone hears about that subtask under its own number.
--
-- The descent carries its depth (that is what orders "nearest the root"), so
-- UNION cannot stop it on a parent ring the way it stops DeleteTask's below; the
-- depth bound does instead — far deeper than any real checklist, and finite.
WITH RECURSIVE down AS (
    SELECT t.id, 0 AS depth FROM tasks t WHERE t.id = $1 AND t.deleted_at IS NULL
    UNION ALL
    SELECT t.id, down.depth + 1 FROM tasks t JOIN down ON t.parent_id = down.id
    WHERE down.depth < 64 AND t.deleted_at IS NULL
),
people AS (
    SELECT ta.user_id, d.id AS task_id, d.depth
    FROM down d JOIN task_assignees ta ON ta.task_id = d.id
    UNION ALL
    SELECT t.created_by AS user_id, d.id AS task_id, d.depth
    FROM down d JOIN tasks t ON t.id = d.id
    WHERE t.created_by IS NOT NULL
)
SELECT DISTINCT ON (p.user_id)
    p.user_id::uuid AS user_id, t.id AS task_id, t.number, t.title, t.board_id
FROM people p JOIN tasks t ON t.id = p.task_id
ORDER BY p.user_id, p.depth, t.position, t.id;

-- DeleteTask removes a task; its subtasks follow by the parent_id cascade.
--
-- Returns the ids it actually removed, which is what the attachment sweep needs:
-- the files under <uploadDir>/<task id>/ are named by nothing but the
-- task_attachments rows this statement has just erased, and a subtask is a task
-- with an id and a directory of its own. Spelling the recursion out rather than
-- leaning on the cascade is what makes those ids observable — the cascade
-- deletes the descendants without ever naming them.
--
-- UNION rather than UNION ALL for the same reason as ArchiveTaskCascade: an
-- install that already grew a ring must get an answer instead of spinning.
-- name: DeleteTask :many
WITH RECURSIVE down AS (
    SELECT t.id FROM tasks t WHERE t.id = $1
    UNION
    SELECT t.id FROM tasks t JOIN down ON t.parent_id = down.id
)
DELETE FROM tasks WHERE tasks.id IN (SELECT id FROM down) RETURNING tasks.id;

-- TaskIDsInColumn / TaskIDsOnBoard / TaskIDsInProject / TaskIDsInWorkspace name the
-- tasks a container delete is about to take by cascade, so their upload directories
-- can be swept the way DeleteTask sweeps its own (purgeTaskUploads).
--
-- DELETE ... RETURNING cannot answer this: it returns the rows of the table named in
-- the statement, never the ones the foreign keys took afterwards. So the ids are read
-- first, inside the same transaction as the delete — a snapshot no concurrent insert
-- can slip a task past.
--
-- Subtasks need no recursion in the three below: every task carries its own
-- board_id/column_id (MoveSubtreeToBoard exists to keep it that way), so a subtree is
-- already wholly inside whichever board, project or workspace holds its root.
--
-- A COLUMN is the exception, and TaskIDsInColumn recurses for it: a subtask is a card
-- of its own — the board draws it, a drag moves it — so it stands in whatever column
-- it was dragged to while its parent stands in another. tasks.parent_id is ON DELETE
-- CASCADE, so deleting the parent's column takes that subtask as well, at any depth
-- and out of a column nobody deleted; without the descent its directory is never
-- swept and the bytes outlive every row that knew where they were.
--
-- UNION rather than UNION ALL for the same reason as TaskAncestorIDs: an install that
-- already grew a ring must get an answer instead of spinning. It also collapses the
-- duplicate ids a subtask standing in the doomed column alongside its parent produces.
-- name: TaskIDsInColumn :many
WITH RECURSIVE down AS (
    SELECT t.id FROM tasks t WHERE t.column_id = $1
    UNION
    SELECT t.id FROM tasks t JOIN down ON t.parent_id = down.id
)
SELECT id FROM down;

-- name: TaskIDsOnBoard :many
SELECT id FROM tasks WHERE board_id = $1;

-- name: TaskIDsInProject :many
SELECT t.id FROM tasks t JOIN boards b ON b.id = t.board_id WHERE b.project_id = $1;

-- name: TaskIDsInWorkspace :many
SELECT t.id FROM tasks t
JOIN boards b ON b.id = t.board_id
JOIN projects p ON p.id = b.project_id
WHERE p.workspace_id = $1;

-- SetTaskDueNotify sets a task's per-task due-notification overrides (NULL = inherit
-- the user default). Used by the card's due popover.
-- name: SetTaskDueNotify :one
UPDATE tasks
SET due_lead_minutes = $2, due_repeat_minutes = $3, due_notify_enabled = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- SetTaskEisenhower pins a task to an Eisenhower-matrix quadrant (NULL = derive the
-- quadrant automatically from priority + due-date). Used by the matrix view's drag.
-- name: SetTaskEisenhower :one
UPDATE tasks
SET eisenhower_quadrant = $2, updated_at = now()
WHERE id = $1
RETURNING *;
