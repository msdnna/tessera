-- Board export and import (#3068).

-- ExportBoardTasks is every live task of a board (cards and subtasks; not
-- archived, not in the trash) with what an export writes beside it: the column,
-- the tag names, the assignees, the parent's number and the stage. Ordered the
-- way the board reads — column by column, card by card, subtasks after their
-- parent's siblings by position.
-- name: ExportBoardTasks :many
SELECT
    t.id, t.number, t.parent_id, t.title, t.description, t.priority,
    t.due_date, t.start_date, t.estimate, t.completed_at, t.created_at,
    c.name AS column_name,
    p.number AS parent_number,
    COALESCE(ms.title, '')::text AS milestone_name,
    tg.tag_names,
    asg.assignee_emails,
    asg.assignee_names
FROM tasks t
JOIN board_columns c ON c.id = t.column_id
LEFT JOIN tasks p ON p.id = t.parent_id AND p.deleted_at IS NULL
LEFT JOIN milestones ms ON ms.id = t.milestone_id
LEFT JOIN LATERAL (
    SELECT COALESCE(array_agg(tg.name ORDER BY tg.name), '{}')::text[] AS tag_names
    FROM task_tags tt JOIN tags tg ON tg.id = tt.tag_id
    WHERE tt.task_id = t.id
) tg ON true
LEFT JOIN LATERAL (
    SELECT
        COALESCE(array_agg(u.email ORDER BY u.email), '{}')::text[] AS assignee_emails,
        COALESCE(array_agg(u.name ORDER BY u.email), '{}')::text[] AS assignee_names
    FROM task_assignees ta JOIN users u ON u.id = ta.user_id
    WHERE ta.task_id = t.id
) asg ON true
WHERE t.board_id = $1
  AND t.deleted_at IS NULL
  AND t.archived_at IS NULL
ORDER BY c.position, (t.parent_id IS NOT NULL), t.position, t.created_at, t.id;

-- ImportBoardColumns lists a board's live columns for matching an imported
-- «Колонка» by name.
-- name: ImportBoardColumns :many
SELECT id, name, name_key, position FROM board_columns
WHERE board_id = $1 AND deleted_at IS NULL
ORDER BY position;
