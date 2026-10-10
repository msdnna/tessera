-- Task subscriptions (#2963): who hears about what happens to a task.
--
-- The author, the assignees and everyone who commented are subscribed
-- implicitly — nothing is written for them, the effective list is computed
-- (ListTaskSubscribers). A row here records an explicit choice only: subscribed
-- = true — someone who is not a participant follows the task; subscribed = false
-- — a participant who unsubscribed and stays out even while still assigned or
-- commenting. Notifications addressed to a person (assigned, mentioned,
-- deleted…) do not look at this table.
CREATE TABLE task_subscriptions (
    task_id    uuid NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subscribed boolean NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, user_id)
);
CREATE INDEX idx_task_subscriptions_user ON task_subscriptions (user_id);
