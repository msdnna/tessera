-- Trigram indexes for workspace search (task #2643). Additive.
--
-- SearchTasks/SearchNotes both filter with a leading-wildcard `ILIKE '%q%'`,
-- which no B-tree index can serve — every search was a seq-scan of `tasks` and
-- `notes` plus a per-row match over `description`/`body`. pg_trgm + GIN fixes
-- exactly that shape without touching the queries: the operator stays `~~*`,
-- so search semantics (substring, case-insensitive) are unchanged.
--
-- Two limits worth knowing before reading a plan:
--   * a trigram index only helps when the pattern holds at least 3 non-wildcard
--     characters — 1-2 char queries still seq-scan. That is inherent to trigrams,
--     not a misconfiguration.
--   * the pattern is built at runtime (`'%' || $2 || '%'`), so the planner has no
--     constant to estimate from. GIN extracts the query trigrams at scan start,
--     so the index is still usable; on small tables the planner may still prefer
--     a seq-scan, which is the right choice there.
--
-- Plain CREATE INDEX, not CONCURRENTLY: golang-migrate wraps each migration in a
-- transaction and CONCURRENTLY cannot run inside one. This takes a write lock on
-- the two tables while the indexes build (seconds at our volume).

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX idx_tasks_title_trgm ON tasks USING gin (title gin_trgm_ops);
CREATE INDEX idx_tasks_description_trgm ON tasks USING gin (description gin_trgm_ops);
CREATE INDEX idx_notes_title_trgm ON notes USING gin (title gin_trgm_ops);
CREATE INDEX idx_notes_body_trgm ON notes USING gin (body gin_trgm_ops);
