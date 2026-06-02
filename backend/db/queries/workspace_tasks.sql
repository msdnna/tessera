-- ListWorkspaceTasks returns every active top-level task across a workspace's
-- boards, with location names and tag/assignee ids — backs "My tasks" / "All
-- tasks" and the home summary (feature #1).
-- name: ListWorkspaceTasks :many
SELECT
    t.*,
    b.name AS board_name,
    p.name AS project_name,
    p.color AS project_color,
    c.name AS column_name,
    c.color AS column_color,
    COALESCE(array_agg(DISTINCT tt.tag_id) FILTER (WHERE tt.tag_id IS NOT NULL), '{}')::uuid[] AS tag_ids,
    COALESCE(array_agg(DISTINCT ta.user_id) FILTER (WHERE ta.user_id IS NOT NULL), '{}')::uuid[] AS assignee_ids
FROM tasks t
JOIN boards b ON b.id = t.board_id
JOIN projects p ON p.id = b.project_id
JOIN board_columns c ON c.id = t.column_id
LEFT JOIN task_tags tt ON tt.task_id = t.id
LEFT JOIN task_assignees ta ON ta.task_id = t.id
WHERE p.workspace_id = $1 AND t.parent_id IS NULL AND t.archived_at IS NULL
GROUP BY t.id, b.name, p.name, p.color, c.name, c.color
ORDER BY t.due_date NULLS LAST, t.created_at DESC;
