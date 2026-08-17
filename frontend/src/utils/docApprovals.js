/**
 * Approval-route rules for the document panel (#2732).
 *
 * These mirror `handlers/document_approvals.go` deliberately: the server is the
 * authority and refuses an out-of-turn signature with 409, but a panel that
 * cannot tell whose turn it is would have to offer everyone a «Подписать»
 * button and explain the refusal afterwards. Keeping the rules pure here means
 * the mirror is testable without a browser or a socket — and when it drifts from
 * the server, it drifts into a disabled button, never into a wrong write.
 */

export const APPROVAL_PENDING = 'pending'
export const APPROVAL_APPROVED = 'approved'
export const APPROVAL_REJECTED = 'rejected'
export const APPROVAL_CANCELLED = 'cancelled'

export const APPROVAL_STATUS_LABEL = {
  [APPROVAL_PENDING]: 'На согласовании',
  [APPROVAL_APPROVED]: 'Согласовано',
  [APPROVAL_REJECTED]: 'Отклонено',
  [APPROVAL_CANCELLED]: 'Отозвано',
}

export const STEP_STATUS_LABEL = {
  [APPROVAL_PENDING]: 'ждёт',
  [APPROVAL_APPROVED]: 'подписал',
  [APPROVAL_REJECTED]: 'отклонил',
}

/** The steps of one route in the order the route is walked. */
export function orderedSteps(approval) {
  const steps = approval?.steps || []
  return [...steps].sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
}

/** The caller's place in a route, or null when they were not asked. */
export function stepForApprover(approval, userId) {
  if (!userId) return null
  return orderedSteps(approval).find((s) => s.approver_id === userId) || null
}

/**
 * Whether `userId` may sign right now.
 *
 * Parallel routes ask everyone at once, so any unsigned step may go. Sequential
 * ones may only be signed by the earliest approver who has not yet decided —
 * that ordering is the entire difference between the two modes.
 */
export function canDecideNow(approval, userId) {
  if (!approval || approval.status !== APPROVAL_PENDING) return false
  const mine = stepForApprover(approval, userId)
  if (!mine || mine.status !== APPROVAL_PENDING) return false
  if (approval.mode !== 'sequential') return true
  const next = orderedSteps(approval).find((s) => s.status === APPROVAL_PENDING)
  return !!next && next.id === mine.id
}

/**
 * How far a route has got: signatures collected out of signatures asked.
 *
 * A rejection counts as decided rather than as progress — the route is over, and
 * "3 из 5" next to «Отклонено» would read as though it were still moving.
 */
export function approvalProgress(approval) {
  const steps = orderedSteps(approval)
  return {
    signed: steps.filter((s) => s.status === APPROVAL_APPROVED).length,
    total: steps.length,
  }
}

/**
 * The state to render one step in: what happened, or — for a pending step — the
 * difference between "ждёт своей очереди" and "сейчас его очередь". Only a
 * sequential route has a current step; in a parallel one everybody's turn is now.
 */
export function stepState(approval, step) {
  if (!step) return 'waiting'
  if (step.status === APPROVAL_APPROVED) return 'signed'
  if (step.status === APPROVAL_REJECTED) return 'rejected'
  if (approval?.status !== APPROVAL_PENDING) return 'waiting'
  if (approval.mode !== 'sequential') return 'current'
  const next = orderedSteps(approval).find((s) => s.status === APPROVAL_PENDING)
  return next && next.id === step.id ? 'current' : 'waiting'
}

/**
 * The document's single answer to "согласован ли он": the newest closed route,
 * unless one is still open. Returns null when the document was never sent for
 * approval, which is different from "not approved" and reads differently in the
 * panel.
 */
export function documentApprovalState(approvals) {
  const list = approvals || []
  if (!list.length) return null
  const open = list.find((a) => a.status === APPROVAL_PENDING)
  return open || list[0]
}

/** Whether a new route may be raised — one open route per document. */
export function canRaiseApproval(approvals) {
  return !(approvals || []).some((a) => a.status === APPROVAL_PENDING)
}
