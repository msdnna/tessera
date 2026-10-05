-- Redesign v2 (#2951): the seeded status columns take the v2 lane palette —
-- «К работе» teal instead of grey, «На рассмотрении» orange instead of purple
-- («В процессе» blue and «Готово» green are unchanged). Only columns still
-- exactly as seeded move: the language-neutral key says it is a default column
-- (renaming clears it) and the colour is still the old default (a colour picked
-- by a person is kept).
UPDATE board_columns SET color = '#0eb0a9' WHERE name_key = 'todo' AND lower(color) = '#9aa0aa';
UPDATE board_columns SET color = '#f0a020' WHERE name_key = 'review' AND lower(color) = '#7c5cff';
