-- No-op: the backfill only materialises what the old runtime fallback already
-- resolved to, and we cannot tell a backfilled pointer from a user-set one.
SELECT 1;
