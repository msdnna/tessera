-- No-op: the columns are owned by migration 0046; this migration only backfilled
-- them where 0046 was skipped, so rolling it back must not drop them.
SELECT 1;
