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

-- name: CloseDocumentApproval :one
UPDATE document_approvals
SET status = $2, closed_at = now()
WHERE id = $1
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
