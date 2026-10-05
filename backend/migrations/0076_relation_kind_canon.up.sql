-- Canonicalise task_relations.kind: 'relate' → 'relates' (задача #2932).
--
-- The quick action «/relate #N» passed its command key straight to applyRelation,
-- so every link made that way was stored under a kind that exists nowhere else in
-- the product. The schema comment on this column, both clients' label maps
-- (`task.relations.kind.*` on web, relKindLabel on Android), the GitLab mapping
-- (gitlab.RelationKind) and the MCP tool all name the same four kinds; a client
-- that meets a fifth prints it verbatim, so those rows read as a bare English
-- «relate» in the «Связи» tab instead of «связана с».
--
-- The uniqueness key is (task_id, related_task_id, kind), so a pair linked both
-- by the command and from the tab is on record twice; the second row survives the
-- ✕ (DeleteRelationBothWays matches on kind too). Drop those before renaming, or
-- the UPDATE below collides with the constraint it is trying to restore.

-- The 'relates' row is the keeper: it is the one both clients can label, and the
-- one the ✕ already removes.
DELETE FROM task_relations r
WHERE r.kind = 'relate'
  AND EXISTS (
      SELECT 1 FROM task_relations o
      WHERE o.task_id = r.task_id
        AND o.related_task_id = r.related_task_id
        AND o.kind = 'relates'
  );

UPDATE task_relations SET kind = 'relates' WHERE kind = 'relate';
