/**
 * Who a workspace manager may act on in the members dialog (#2932).
 *
 * These mirror `handlers/workspaces.go` — AddMember, UpdateMemberRole and
 * RemoveMember all refuse two targets, in this order: the workspace owner
 * ("cannot change the workspace owner's role" / "cannot remove the workspace
 * owner") and the caller themselves ("cannot change your own role" /
 * "cannot remove yourself from the workspace"). The dialog already knew about
 * the owner and rendered a read-only badge for them; it did not know who was
 * *looking*, so an admin's own row carried the same role select and the same
 * trash icon as everybody else's — one tap from losing members, roles,
 * invitations and settings, with no other admin necessarily around to give them
 * back.
 *
 * Pure and outside the component for the usual reason: a rule that only ever
 * runs inside a modal is a rule no test can hold still. The server stays the
 * authority — when this drifts from it the dialog offers a control the server
 * refuses, never the other way round.
 */

/** Locks in the order the server applies them: '' means the row is actionable. */
export const LOCK_NONE = ''
export const LOCK_OWNER = 'owner'
export const LOCK_SELF = 'self'

/**
 * Why a member row's controls are unavailable, as one of the LOCK_* codes.
 *
 * Owner first, matching the handler: the owner rule holds no matter who is
 * asking, so an owner looking at their own row hears that their role is
 * immutable — not that they may not edit themselves, which would suggest
 * somebody else could.
 *
 * @param {{user_id?: string, role?: string}|null|undefined} member
 * @param {{ownerId?: string, selfId?: string}} who
 * @returns {''|'owner'|'self'}
 */
export function memberRowLock(member, { ownerId, selfId } = {}) {
  if (!member) return LOCK_NONE
  // `role === 'owner'` is what the list actually carries; ownerId is accepted
  // too so a caller holding the workspace record does not have to trust the row.
  if (member.role === 'owner' || (ownerId && member.user_id === ownerId)) return LOCK_OWNER
  if (selfId && member.user_id === selfId) return LOCK_SELF
  return LOCK_NONE
}

/** Whether this row's role select should be live. */
export function canChangeMemberRole(member, who) {
  return memberRowLock(member, who) === LOCK_NONE
}

/** Whether this row's remove button should be offered. */
export function canRemoveMember(member, who) {
  return memberRowLock(member, who) === LOCK_NONE
}

const normalizeEmail = (value) =>
  String(value ?? '')
    .trim()
    .toLowerCase()

/**
 * Why adding this address would be refused, before the request is made.
 *
 * The add input is the same operation by another door: CreateMembership is an
 * upsert, so a manager who types an address already on the list changes that
 * member's role instead of adding anyone — and typing *their own* address, with
 * the role select on its "member" default, demoted them and answered with a
 * green "Участник добавлен". The server now refuses both, so this only decides
 * whether the reader is told before or after a pointless round trip.
 *
 * Addresses are compared lowercased and trimmed because GetUserByEmail is
 * `strings.ToLower(...)`: "Admin@example.com " and "admin@example.com" are the
 * same account, and a guard that missed that would be a guard in name only.
 *
 * @param {string} email the address typed into the invite field
 * @param {Array<{user_id?: string, email?: string, role?: string}>} members
 * @param {{ownerId?: string, selfId?: string, selfEmail?: string}} who
 * @returns {''|'owner'|'self'}
 */
export function addMemberRefusal(email, members, who = {}) {
  const typed = normalizeEmail(email)
  if (!typed) return LOCK_NONE
  const { ownerId, selfId, selfEmail } = who
  const row = (members || []).find((m) => normalizeEmail(m?.email) === typed)
  // The owner is refused whether or not the caller can see their row, so their
  // address is checked against the list first and the id second.
  if (row && memberRowLock(row, { ownerId, selfId }) !== LOCK_NONE) {
    return memberRowLock(row, { ownerId, selfId })
  }
  if (selfEmail && typed === normalizeEmail(selfEmail)) return LOCK_SELF
  return LOCK_NONE
}
