package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.Member

/*
 * Who the members dialog may act on (#2932) — the app's copy of the web
 * `utils/members.js`.
 *
 * The dialog itself is open to everyone: ListMembers sits behind `requireMember`
 * and the button that opens it is shown to every member, because "who else is in
 * this workspace" is worth reading whoever asks. Everything the dialog *writes*
 * — AddMember, UpdateMemberRole, RemoveMember, and the invitation list it shows
 * alongside — sits behind `requireManager` instead, and on top of that the
 * handlers refuse two targets in a fixed order: the workspace owner, then the
 * caller themselves.
 *
 * Pure and outside the screen for the usual reason: a rule that only ever runs
 * inside a Compose dialog is a rule no test can hold still. The server stays the
 * authority — when this drifts from it the dialog offers a control the server
 * refuses, never the other way round.
 */

/** Workspace roles `requireManager` lets through. */
val MEMBER_MANAGER_ROLES = setOf("owner", "admin")

/** Why a member row's controls are unavailable; [NONE] means the row is actionable. */
enum class MemberLock {
    /** Actionable. */
    NONE,

    /** The viewer manages nobody here — `requireManager` refuses every row alike. */
    VIEWER,

    /** The workspace owner: their role is immutable and they cannot be removed. */
    OWNER,

    /** The viewer's own row: no self-demotion, no self-removal. */
    SELF,
}

/** What a row should call itself, regardless of what it may do. */
enum class MemberBadge {
    /** The workspace owner. */
    OWNER,

    /** The viewer. */
    YOU,

    /** Anybody else — the row shows their role. */
    ROLE,
}

/**
 * May the viewer manage this workspace's members at all.
 *
 * Read off the roster the dialog has just loaded rather than off the workspace
 * record: `ListMembers` answers with my own row too, so the same response that
 * fills the dialog also says what I am here. The web client reads `my_role` from
 * its workspace store instead — it has one; ours has no such field, and a role
 * cached at sidebar-load time would go stale exactly when someone else changes
 * it, which is the moment the dialog is open.
 */
fun canManageMembers(members: List<Member>, meId: String): Boolean =
    meId.isNotBlank() && members.firstOrNull { it.userId == meId }?.role in MEMBER_MANAGER_ROLES

/**
 * Why this row's controls are refused, in the order the server applies them.
 *
 * Viewer first, matching the handlers: `requireManager` is the gate every one of
 * these routes passes before it looks at the target at all, so a plain member is
 * refused on their own row exactly as on anybody else's.
 *
 * Owner before self, matching the handler: the owner rule holds no matter who is
 * asking, so an owner looking at their own row hears that their role is
 * immutable — not that they may not edit themselves, which would suggest
 * somebody else could.
 *
 * [canManage] defaults to true so a caller that has not been taught about the
 * viewer rule keeps drifting in the one safe direction: offering a control the
 * server refuses, never hiding one it would allow.
 */
fun memberRowLock(member: Member?, meId: String = "", canManage: Boolean = true): MemberLock {
    if (member == null) return MemberLock.NONE
    if (!canManage) return MemberLock.VIEWER
    if (member.role == "owner") return MemberLock.OWNER
    if (meId.isNotBlank() && member.userId == meId) return MemberLock.SELF
    return MemberLock.NONE
}

/**
 * What a row says about itself.
 *
 * Deliberately not derived from the lock: the lock answers "why is this
 * refused", which for a plain member is the same answer on every row, and a
 * roster where all four rows read «Участник» tells the reader less than the one
 * they had before. So the badge keeps naming the row — owner, you, or the role.
 */
fun memberRowBadge(member: Member?, meId: String = ""): MemberBadge = when {
    member == null -> MemberBadge.ROLE
    member.role == "owner" -> MemberBadge.OWNER
    meId.isNotBlank() && member.userId == meId -> MemberBadge.YOU
    else -> MemberBadge.ROLE
}

/** Whether this row's role badge should open the role menu. */
fun canChangeMemberRole(member: Member?, meId: String = "", canManage: Boolean = true): Boolean =
    memberRowLock(member, meId, canManage) == MemberLock.NONE

/** Whether this row should offer the remove button. */
fun canRemoveMember(member: Member?, meId: String = "", canManage: Boolean = true): Boolean =
    memberRowLock(member, meId, canManage) == MemberLock.NONE
