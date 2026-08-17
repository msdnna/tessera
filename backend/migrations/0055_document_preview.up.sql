-- Documents are listed as tiles with a preview (#2726 review feedback), and the
-- list query deliberately does not select `content` — with the editor from D2 a
-- document is hundreds of KB of ProseMirror JSON, and shipping every one of them
-- to render a grid would defeat the point of the tiles.
--
-- The preview is therefore materialised: the content endpoint derives plain text
-- from the tree it is already parsing and stores a truncated copy here. Doing it
-- in SQL instead would mean walking the jsonb tree on every list request.
ALTER TABLE documents ADD COLUMN preview text NOT NULL DEFAULT '';
