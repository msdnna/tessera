-- Approval protocols (#2732, D7 of #2718) — point 2 of the parent task, second
-- half: маршрут согласования, статусы, подписи.
--
-- The decision the whole table hangs on: a route is raised against a *version
-- snapshot* (D6), never against the live document. "Документ согласован" has to
-- mean that a specific text was agreed; a route bound to the mutable row would be
-- quietly invalidated by the next autosave, and the approvers would have signed
-- something nobody can reconstruct afterwards. So version_id is NOT NULL and the
-- handler pins the snapshot as manual, which also takes it out of retention.
CREATE TABLE document_approvals (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    -- RESTRICT rather than CASCADE: pruning history must never be able to delete
    -- the text that was signed. Nothing deletes versions except retention, and
    -- retention skips manual ones — this is the backstop under that rule.
    version_id  uuid NOT NULL REFERENCES document_versions(id) ON DELETE RESTRICT,
    title       text NOT NULL DEFAULT '',
    -- Derived from the steps, stored anyway: every list that shows a document
    -- wants the badge, and recomputing it means walking the route each time.
    status      text NOT NULL DEFAULT 'pending',
    -- sequential: the next approver is asked only once the previous one signed —
    -- the usual paper route. parallel: everyone is asked at once.
    mode        text NOT NULL DEFAULT 'sequential',
    created_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    closed_at   timestamptz,
    CONSTRAINT document_approvals_status_valid
        CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    CONSTRAINT document_approvals_mode_valid
        CHECK (mode IN ('sequential', 'parallel'))
);

CREATE INDEX idx_document_approvals_document ON document_approvals (document_id, created_at DESC);
CREATE INDEX idx_document_approvals_version ON document_approvals (version_id);

-- One approver's place in the route, and their signature once they decide.
CREATE TABLE document_approval_steps (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    approval_id uuid NOT NULL REFERENCES document_approvals(id) ON DELETE CASCADE,
    -- SET NULL on a deleted account, like every other authored row here — but the
    -- protocol still has to read as a protocol afterwards, which is what
    -- approver_name below is for. RESTRICT was the alternative and it is worse:
    -- it would make a years-old signed route block deleting an employee.
    approver_id uuid REFERENCES users(id) ON DELETE SET NULL,
    -- Who was asked, captured when the route was raised. A signed protocol that
    -- turns anonymous because someone left the company is not a protocol.
    approver_name text NOT NULL DEFAULT '',
    -- Order in the route. Meaningful in sequential mode; in parallel mode it is
    -- only the order the panel lists people in.
    position    integer NOT NULL DEFAULT 0,
    status      text NOT NULL DEFAULT 'pending',
    comment     text NOT NULL DEFAULT '',
    -- The signature proper: the name the approver confirmed with at the moment of
    -- signing. Not a cryptographic signature — this is an internal протокол
    -- согласования, and what it has to answer is "кто и когда согласовал какую
    -- редакцию", which id + decided_at + the route's version_id already pin down.
    signature   text NOT NULL DEFAULT '',
    decided_at  timestamptz,
    CONSTRAINT document_approval_steps_status_valid
        CHECK (status IN ('pending', 'approved', 'rejected')),
    -- Asking the same person twice in one route has no meaning: their single
    -- decision would have to satisfy two steps, and in sequential mode the route
    -- would stall on the second one forever.
    CONSTRAINT document_approval_steps_unique_approver UNIQUE (approval_id, approver_id)
);

CREATE INDEX idx_document_approval_steps_approval ON document_approval_steps (approval_id, position);
CREATE INDEX idx_document_approval_steps_approver ON document_approval_steps (approver_id) WHERE approver_id IS NOT NULL;
