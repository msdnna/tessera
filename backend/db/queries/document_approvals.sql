-- name: CreateDocumentApproval :one
INSERT INTO document_approvals (document_id, version_id, title, mode, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- ListDocumentApprovals returns a document's routes, newest first. The revision
-- of the pinned snapshot is joined in because the panel's first job is to say
-- *which text* each protocol is about — an id would not answer that.
-- name: ListDocumentApprovals :many
SELECT a.id, a.document_id, a.version_id, a.title, a.status, a.mode,
       a.created_by, a.created_at, a.closed_at,
       v.revision AS version_revision,
       u.name AS created_by_name
FROM document_approvals a
JOIN document_versions v ON v.id = a.version_id
LEFT JOIN users u ON u.id = a.created_by
WHERE a.document_id = $1
ORDER BY a.created_at DESC;

-- name: GetDocumentApproval :one
SELECT * FROM document_approvals WHERE id = $1;

-- PendingDocumentApproval finds the document's open route, if any. Raising a
-- second one while the first is still out is refused: two routes would be
-- collecting signatures on two different revisions at once, and "документ
-- согласован" would stop having a single answer.
-- name: PendingDocumentApproval :one
SELECT * FROM document_approvals
WHERE document_id = $1 AND status = 'pending'
ORDER BY created_at DESC
LIMIT 1;

-- LockDocumentApprovalSlot serialises everything that raises a route on one
-- document. The "one open route" rule is a read followed by an insert, so
-- without a lock two simultaneous requests both read "none open" and both
-- insert — and the rule PendingDocumentApproval exists to enforce is broken by
-- the very pair of calls it was written for. The document row is the slot
-- because the route being guarded against does not exist yet: locking a row
-- that is not there blocks nobody.
-- name: LockDocumentApprovalSlot :one
SELECT id FROM documents WHERE id = $1 FOR UPDATE;

-- LockDocumentApproval takes the route for a read-modify-write on its steps.
-- Deciding is exactly that: read the steps, write one, recompute the route's
-- status from the whole list. Two approvers signing at the same moment each
-- recompute from a list missing the other's signature, so a route whose last
-- two steps land together stays 'pending' with nothing left to sign — and the
-- rule above then refuses to raise a replacement.
-- name: LockDocumentApproval :one
SELECT * FROM document_approvals WHERE id = $1 FOR UPDATE;

-- Closing is guarded on 'pending' so a cancellation and a final signature that
-- cross cannot overwrite each other's outcome.
-- name: CloseDocumentApproval :one
UPDATE document_approvals
SET status = $2, closed_at = now()
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: CreateDocumentApprovalStep :one
INSERT INTO document_approval_steps (approval_id, approver_id, approver_name, position)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- ListDocumentApprovalStepsByDocument reads every step of every route on one
-- document in a single query, the same shape ListDocumentComments uses: the
-- panel renders routes with their steps, and a query per route would make the
-- number of round trips depend on how long the document has been in review.
--
-- approver_name comes from the step, not from the join: it is what the protocol
-- recorded, and it has to survive the account being renamed or deleted. The join
-- only supplies the avatar-grade email for people who still exist.
-- name: ListDocumentApprovalStepsByDocument :many
SELECT s.id, s.approval_id, s.approver_id, s.approver_name, s.position, s.status,
       s.comment, s.signature, s.decided_at,
       u.email AS approver_email
FROM document_approval_steps s
JOIN document_approvals a ON a.id = s.approval_id
LEFT JOIN users u ON u.id = s.approver_id
WHERE a.document_id = $1
ORDER BY s.approval_id, s.position;

-- name: ListDocumentApprovalSteps :many
SELECT * FROM document_approval_steps
WHERE approval_id = $1
ORDER BY position;

-- name: DecideDocumentApprovalStep :one
UPDATE document_approval_steps
SET status = $2, comment = $3, signature = $4, decided_at = now()
WHERE id = $1
RETURNING *;

-- CancelApprovalsAcrossWorkspaces closes every open route on a project's
-- documents that the project's transfer has just made unsignable.
--
-- CreateDocumentApproval refuses an approver who is not a member of the
-- document's workspace, and says why: "someone who cannot open it can only ever
-- stall it". A transfer walks the documents into another workspace and leaves
-- the steps pointing at the people the *source* workspace asked, so the door's
-- rule is broken from behind — and unlike the other cross-workspace links, this
-- one has no reader to sever. There is nothing to cut: the harm is that the
-- route survives intact and can never be finished. loadDocumentApproval
-- authorizes on the document's workspace, so the stranded approver is answered
-- 403, and no endpoint adds or replaces a step in a route already raised. The
-- document keeps its «на согласовании» badge until a manager of a workspace that
-- never raised the route happens to notice and cancel it by hand — and in
-- sequential mode everyone queued behind the stranded step waits on it forever.
--
-- Cancelling is the whole remedy: the protocol is kept, as CancelDocumentApproval
-- keeps it ("the record that a document was sent for approval and pulled back is
-- exactly the kind of thing a журнал согласований exists to keep"), and the team
-- that now owns the document can raise a fresh route with its own people — which
-- the one-open-route rule would have refused while this one hung.
--
-- Only a route with a *pending* step held by a stranger is touched. One whose
-- remaining approvers are members of both sides is signable exactly as before and
-- must not be voided, and a step already decided is a signature, not a debt. An
-- approver_id of NULL is a deleted account — a route stalled for its own reasons
-- long before this transfer, and not this query's business to close.
-- name: CancelApprovalsAcrossWorkspaces :execrows
UPDATE document_approvals a
SET status = 'cancelled', closed_at = now()
WHERE a.status = 'pending'
  AND a.document_id IN (SELECT d.id FROM documents d WHERE d.project_id = $1)
  AND EXISTS (
    SELECT 1 FROM document_approval_steps s
    WHERE s.approval_id = a.id
      AND s.status = 'pending'
      AND s.approver_id IS NOT NULL
      AND NOT EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.workspace_id = $2 AND m.user_id = s.approver_id
      )
  );
