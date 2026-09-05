package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.DocumentApproval
import website.msdnna.tessera.data.model.DocumentApprovalMode
import website.msdnna.tessera.data.model.DocumentApprovalStatus
import website.msdnna.tessera.data.model.DocumentApprovalStep

// Approval-route rules for the links panel (#2732, §7 of #2894) — the app's copy
// of the web `utils/docApprovals.js`.
//
// The server is the authority and refuses an out-of-turn signature with 409, but
// a panel that cannot tell whose turn it is would have to offer everyone a
// «Подписать» button and explain the refusal afterwards. Keeping the rules pure
// here means the mirror is testable without a screen — and when it drifts from
// the server it drifts into a disabled button, never into a wrong write.

/** How one step of a route reads: what happened, or whose turn it is. */
enum class DocStepState {
    /** Signed off. */
    SIGNED,

    /** Refused — the route is over. */
    REJECTED,

    /** Their turn is now. */
    CURRENT,

    /** Asked, but not yet their turn (sequential routes only). */
    WAITING,
}

/** Signatures collected out of signatures asked. */
data class DocApprovalProgress(val signed: Int, val total: Int)

/** The steps of one route in the order the route is walked. */
fun orderedApprovalSteps(approval: DocumentApproval?): List<DocumentApprovalStep> =
    approval?.steps.orEmpty().sortedBy { it.position }

/** The caller's place in a route, or null when they were not asked. */
fun approvalStepFor(approval: DocumentApproval?, userId: String?): DocumentApprovalStep? {
    if (userId.isNullOrBlank()) return null
    return orderedApprovalSteps(approval).firstOrNull { it.approverId == userId }
}

/**
 * Whether [userId] may sign right now.
 *
 * Parallel routes ask everyone at once, so any unsigned step may go. Sequential
 * ones may only be signed by the earliest approver who has not yet decided —
 * that ordering is the entire difference between the two modes.
 */
fun canDecideNow(approval: DocumentApproval?, userId: String?): Boolean {
    if (approval == null || approval.status != DocumentApprovalStatus.PENDING) return false
    val mine = approvalStepFor(approval, userId) ?: return false
    if (mine.status != DocumentApprovalStatus.PENDING) return false
    if (approval.mode != DocumentApprovalMode.SEQUENTIAL) return true
    val next = nextPendingStep(approval) ?: return false
    return next.id == mine.id
}

/**
 * How far a route has got.
 *
 * A rejection counts as decided rather than as progress — the route is over, and
 * «3 из 5» next to «Отклонено» would read as though it were still moving.
 */
fun approvalProgress(approval: DocumentApproval?): DocApprovalProgress {
    val steps = orderedApprovalSteps(approval)
    return DocApprovalProgress(
        signed = steps.count { it.status == DocumentApprovalStatus.APPROVED },
        total = steps.size,
    )
}

/**
 * The state to render one step in: what happened, or — for a pending step — the
 * difference between «ждёт своей очереди» and «сейчас его очередь». Only a
 * sequential route has a current step; in a parallel one everybody's turn is now.
 */
fun approvalStepState(approval: DocumentApproval?, step: DocumentApprovalStep?): DocStepState {
    if (step == null) return DocStepState.WAITING
    if (step.status == DocumentApprovalStatus.APPROVED) return DocStepState.SIGNED
    if (step.status == DocumentApprovalStatus.REJECTED) return DocStepState.REJECTED
    if (approval?.status != DocumentApprovalStatus.PENDING) return DocStepState.WAITING
    if (approval.mode != DocumentApprovalMode.SEQUENTIAL) return DocStepState.CURRENT
    val next = nextPendingStep(approval)
    return if (next != null && next.id == step.id) DocStepState.CURRENT else DocStepState.WAITING
}

/**
 * The document's single answer to «согласован ли он»: the newest closed route,
 * unless one is still open. Returns null when the document was never sent for
 * approval, which is different from «not approved» and reads differently in the
 * panel.
 */
fun documentApprovalState(approvals: List<DocumentApproval>): DocumentApproval? {
    if (approvals.isEmpty()) return null
    return approvals.firstOrNull { it.status == DocumentApprovalStatus.PENDING } ?: approvals.first()
}

/** Whether a new route may be raised — one open route per document. */
fun canRaiseApproval(approvals: List<DocumentApproval>): Boolean =
    approvals.none { it.status == DocumentApprovalStatus.PENDING }

/**
 * Task links pinned to a block, counted per block.
 *
 * The reader marks a block that has tasks hanging off it, and a lookup per
 * paragraph would otherwise walk the whole list on every row.
 */
fun docLinksByBlock(links: List<website.msdnna.tessera.data.model.DocumentTaskLink>): Map<String, Int> {
    val out = LinkedHashMap<String, Int>()
    for (l in links) {
        if (l.blockId.isBlank()) continue
        out[l.blockId] = (out[l.blockId] ?: 0) + 1
    }
    return out
}

/** The earliest approver who has not yet decided — the only one a sequential
 *  route will accept a signature from. */
private fun nextPendingStep(approval: DocumentApproval): DocumentApprovalStep? =
    orderedApprovalSteps(approval).firstOrNull { it.status == DocumentApprovalStatus.PENDING }
