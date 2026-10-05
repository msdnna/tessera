-- No-op on purpose: the up migration folds a misspelt kind into the canonical one
-- and deletes the rows that duplicated an existing link. Neither is recoverable —
-- nothing records which 'relates' rows used to be 'relate', and re-spelling them
-- would only put the untranslatable kind back. Rolling back the schema is fine;
-- rolling back this data is not.
SELECT 1;
