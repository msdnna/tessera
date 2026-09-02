-- No-op on purpose: the up migration deletes duplicated GitLab-sourced comments
-- and rewrites gl_note_id to one canonical spelling. Neither is recoverable —
-- the deleted rows are gone and the original class name ("DiscussionNote" vs
-- "Note") is not recorded anywhere. Rolling back the schema is fine; rolling back
-- this data is not, and pretending otherwise would be worse than saying so.
SELECT 1;
