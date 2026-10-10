-- Task subscriptions (#2963). The effective subscribers of a task are its
-- implicit participants — the author, the assignees, the comment authors —
-- minus those who unsubscribed, plus those who subscribed explicitly; only
-- members of the task's workspace count (someone who left stops hearing).

-- name: ListTaskSubscribers :many
WITH implicit AS (
    SELECT t.created_by AS user_id FROM tasks t
    WHERE t.id = @task_id AND t.created_by IS NOT NULL AND t.deleted_at IS NULL
    UNION
    SELECT ta.user_id FROM task_assignees ta WHERE ta.task_id = @task_id
    UNION
    SELECT tc.author_id FROM task_comments tc WHERE tc.task_id = @task_id AND tc.author_id IS NOT NULL
)
SELECT u.id, u.email, u.name
FROM users u
WHERE EXISTS (
        SELECT 1 FROM memberships m WHERE m.user_id = u.id AND m.workspace_id = @workspace_id
    )
  AND (
        (u.id IN (SELECT i.user_id FROM implicit i)
         AND NOT EXISTS (
            SELECT 1 FROM task_subscriptions s
            WHERE s.task_id = @task_id AND s.user_id = u.id AND NOT s.subscribed
         ))
        OR EXISTS (
            SELECT 1 FROM task_subscriptions s
            WHERE s.task_id = @task_id AND s.user_id = u.id AND s.subscribed
        )
  )
ORDER BY u.name, u.id;

-- name: SetTaskSubscription :exec
INSERT INTO task_subscriptions (task_id, user_id, subscribed)
VALUES (@task_id, @user_id, @subscribed)
ON CONFLICT (task_id, user_id)
DO UPDATE SET subscribed = EXCLUDED.subscribed, updated_at = now();
