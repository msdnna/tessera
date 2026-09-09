-- Canonicalise task_comments.gl_note_id and remove the duplicate comments the two
-- spellings produced (task #2865).
--
-- A comment pushed from Tessera is created via POST /issues/<iid>/discussions and
-- stored as "gid://gitlab/Note/<id>". The next pull reads the very same note back
-- through GraphQL, where a note inside a discussion carries GitLab's own class
-- name — "gid://gitlab/DiscussionNote/<id>". gl_note_id is unique and upserted
-- with ON CONFLICT, so the second spelling missed the conflict and inserted a
-- GitLab-sourced copy of the user's own comment (author_id NULL, hence not
-- editable in the UI).
--
-- The numeric tail is the note's identity; the class name is not. So group by the
-- tail, keep the local comment, drop the imported copy, and normalise what is left.
-- Only a row with author_id IS NULL is ever deleted — a comment a human wrote is
-- never a candidate.

-- ── 1. duplicates whose local twin never got a gid ───────────────────────────
-- The second failure mode: the pull raced the push, and the body-based fallback
-- that should have claimed the local comment compared the raw GitLab body against
-- the rewritten local one, so a comment with an attachment never matched. The
-- local row is left with gl_note_id NULL, which puts it out of reach of the tail
-- grouping below — pair those by body instead and hand the gid to the local row.
-- Bodies are stored already rewritten on both sides, so this compares like with like.
CREATE TEMP TABLE _gl_body_pairs AS
SELECT DISTINCT ON (loc.id)
       loc.id AS keeper_id, imp.id AS dup_id,
       imp.gl_note_id AS gid, imp.gl_discussion_id AS disc
FROM task_comments imp
JOIN task_comments loc
  ON loc.task_id = imp.task_id
 AND loc.body = imp.body
 AND loc.author_id IS NOT NULL
 AND loc.gl_note_id IS NULL
WHERE imp.author_id IS NULL
  AND imp.gl_note_id IS NOT NULL
ORDER BY loc.id, imp.created_at;

-- One import can only be claimed by one local comment; an ambiguous match (two
-- identical local comments) is left alone rather than guessed at.
DELETE FROM _gl_body_pairs p
USING (SELECT dup_id FROM _gl_body_pairs GROUP BY dup_id HAVING count(*) > 1) amb
WHERE p.dup_id = amb.dup_id;

UPDATE task_comments c
SET parent_id = p.keeper_id, updated_at = now()
FROM _gl_body_pairs p
WHERE c.parent_id = p.dup_id;

DELETE FROM task_comments c USING _gl_body_pairs p WHERE c.id = p.dup_id;

UPDATE task_comments c
SET gl_note_id = p.gid, gl_discussion_id = p.disc, updated_at = now()
FROM _gl_body_pairs p
WHERE c.id = p.keeper_id;

DROP TABLE _gl_body_pairs;

-- ── 2. duplicates that differ only in the class name ─────────────────────────
CREATE TEMP TABLE _gl_note_canon AS
SELECT c.id, c.task_id, c.author_id, c.created_at,
       regexp_replace(c.gl_note_id, '^.*/', '') AS tail
FROM task_comments c
WHERE c.gl_note_id IS NOT NULL
  AND regexp_replace(c.gl_note_id, '^.*/', '') ~ '^[0-9]+$';

-- Survivor per (task, note): the local comment if there is one, else the oldest.
CREATE TEMP TABLE _gl_note_keep AS
SELECT DISTINCT ON (task_id, tail) task_id, tail, id AS keeper_id
FROM _gl_note_canon
ORDER BY task_id, tail, (author_id IS NULL), created_at;

CREATE TEMP TABLE _gl_note_drop AS
SELECT n.id, k.keeper_id
FROM _gl_note_canon n
JOIN _gl_note_keep k ON k.task_id = n.task_id AND k.tail = n.tail
WHERE n.id <> k.keeper_id
  AND n.author_id IS NULL;

-- Replies threaded under the duplicate move to the survivor. parent_id is
-- ON DELETE SET NULL, so without this they would silently become root comments.
UPDATE task_comments c
SET parent_id = d.keeper_id, updated_at = now()
FROM _gl_note_drop d
WHERE c.parent_id = d.id;

DELETE FROM task_comments c USING _gl_note_drop d WHERE c.id = d.id;

-- Normalise the spelling of everything that survived. The NOT EXISTS guard keeps
-- the unique index happy in the pathological case where the canonical form is
-- already held by another row (e.g. the same note imported under two tasks).
UPDATE task_comments c
SET gl_note_id = 'gid://gitlab/Note/' || n.tail
FROM _gl_note_canon n
WHERE c.id = n.id
  AND c.gl_note_id IS DISTINCT FROM 'gid://gitlab/Note/' || n.tail
  AND NOT EXISTS (
      SELECT 1 FROM task_comments o
      WHERE o.gl_note_id = 'gid://gitlab/Note/' || n.tail
        AND o.id <> c.id
  );

DROP TABLE _gl_note_drop;
DROP TABLE _gl_note_keep;
DROP TABLE _gl_note_canon;
