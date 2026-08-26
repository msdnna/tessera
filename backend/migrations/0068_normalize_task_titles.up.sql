-- One-shot repair for titles that already carry a newline. Quick-add in a column
-- listened on keyup.enter while the textarea inserts its newline on keydown, so
-- Enter with the caret mid-string left a real "\n" inside the title (#2813). The
-- card renders the title as HTML (newline shows as a space) but the modal keeps
-- it in an <input>, which strips newlines from value — same task, two readings.
--
-- The event handler and both client- and server-side normalization are fixed in
-- the same change; this only cleans up what the old path already stored.
UPDATE tasks
SET    title = btrim(regexp_replace(title, '\s+', ' ', 'g'))
WHERE  title <> btrim(regexp_replace(title, '\s+', ' ', 'g'));
