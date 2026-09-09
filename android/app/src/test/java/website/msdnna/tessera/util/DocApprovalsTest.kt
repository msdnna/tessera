package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.DocumentApproval
import website.msdnna.tessera.data.model.DocumentApprovalMode
import website.msdnna.tessera.data.model.DocumentApprovalStatus
import website.msdnna.tessera.data.model.DocumentApprovalStep
import website.msdnna.tessera.data.model.DocumentTaskLink

/**
 * Approval-route rules of the links panel (#2732, §7 of #2894).
 *
 * This is where the panel can be quietly wrong: offering «Подписать» to someone
 * whose turn has not come (the server answers 409 and the button looks broken),
 * counting a rejected route as progress, or letting a second route be raised
 * over an open one. The rules are pure, so none of it needs a screen.
 */
class DocApprovalsTest {
    private fun step(
        id: String,
        approver: String,
        position: Int,
        status: String = DocumentApprovalStatus.PENDING,
    ) = DocumentApprovalStep(
        id = id,
        approverId = approver,
        approverName = approver,
        position = position,
        status = status,
    )

    private fun route(
        id: String = "a1",
        status: String = DocumentApprovalStatus.PENDING,
        mode: String = DocumentApprovalMode.SEQUENTIAL,
        steps: List<DocumentApprovalStep> = emptyList(),
    ) = DocumentApproval(id = id, status = status, mode = mode, steps = steps)

    @Test
    fun `steps come out in route order, whatever order the server sent them`() {
        val a = route(
            steps = listOf(
                step("s3", "c", position = 2),
                step("s1", "a", position = 0),
                step("s2", "b", position = 1),
            ),
        )

        assertThat(orderedApprovalSteps(a).map { it.approverId }).containsExactly("a", "b", "c").inOrder()
    }

    @Test
    fun `a sequential route only lets the earliest undecided approver sign`() {
        val a = route(
            steps = listOf(
                step("s1", "a", position = 0, status = DocumentApprovalStatus.APPROVED),
                step("s2", "b", position = 1),
                step("s3", "c", position = 2),
            ),
        )

        assertThat(canDecideNow(a, "b")).isTrue()
        // c was asked, but not yet — the server would answer 409, so the panel
        // must not offer the button.
        assertThat(canDecideNow(a, "c")).isFalse()
        // a already signed.
        assertThat(canDecideNow(a, "a")).isFalse()
        // Someone who was never on the route.
        assertThat(canDecideNow(a, "d")).isFalse()
    }

    @Test
    fun `a parallel route asks everyone at once`() {
        val a = route(
            mode = DocumentApprovalMode.PARALLEL,
            steps = listOf(
                step("s1", "a", position = 0),
                step("s2", "b", position = 1),
                step("s3", "c", position = 2, status = DocumentApprovalStatus.APPROVED),
            ),
        )

        assertThat(canDecideNow(a, "a")).isTrue()
        assertThat(canDecideNow(a, "b")).isTrue()
        assertThat(canDecideNow(a, "c")).isFalse()
    }

    @Test
    fun `a closed route is signed by nobody`() {
        val steps = listOf(step("s1", "a", position = 0))
        for (status in listOf(
            DocumentApprovalStatus.APPROVED,
            DocumentApprovalStatus.REJECTED,
            DocumentApprovalStatus.CANCELLED,
        )) {
            assertThat(canDecideNow(route(status = status, steps = steps), "a")).isFalse()
        }
    }

    @Test
    fun `nobody signs on behalf of an unidentified caller`() {
        val a = route(mode = DocumentApprovalMode.PARALLEL, steps = listOf(step("s1", "a", position = 0)))

        // The step's approver_id is null on a route whose approver's account was
        // removed; a blank «me» must not match it.
        assertThat(canDecideNow(a, null)).isFalse()
        assertThat(canDecideNow(a, "")).isFalse()
        val orphan = route(
            mode = DocumentApprovalMode.PARALLEL,
            steps = listOf(DocumentApprovalStep(id = "s1", approverId = null, status = DocumentApprovalStatus.PENDING)),
        )
        assertThat(canDecideNow(orphan, "")).isFalse()
    }

    @Test
    fun `a rejection is decided, not progress`() {
        val a = route(
            status = DocumentApprovalStatus.REJECTED,
            steps = listOf(
                step("s1", "a", position = 0, status = DocumentApprovalStatus.APPROVED),
                step("s2", "b", position = 1, status = DocumentApprovalStatus.REJECTED),
                step("s3", "c", position = 2),
            ),
        )

        val p = approvalProgress(a)
        assertThat(p.signed).isEqualTo(1)
        assertThat(p.total).isEqualTo(3)
    }

    @Test
    fun `only a sequential route has a step whose turn is now`() {
        val steps = listOf(
            step("s1", "a", position = 0, status = DocumentApprovalStatus.APPROVED),
            step("s2", "b", position = 1),
            step("s3", "c", position = 2),
        )
        val sequential = route(steps = steps)

        assertThat(approvalStepState(sequential, steps[0])).isEqualTo(DocStepState.SIGNED)
        assertThat(approvalStepState(sequential, steps[1])).isEqualTo(DocStepState.CURRENT)
        assertThat(approvalStepState(sequential, steps[2])).isEqualTo(DocStepState.WAITING)

        val parallel = route(mode = DocumentApprovalMode.PARALLEL, steps = steps)
        assertThat(approvalStepState(parallel, steps[1])).isEqualTo(DocStepState.CURRENT)
        assertThat(approvalStepState(parallel, steps[2])).isEqualTo(DocStepState.CURRENT)
    }

    @Test
    fun `a pending step of a closed route waits rather than asks`() {
        val steps = listOf(
            step("s1", "a", position = 0, status = DocumentApprovalStatus.REJECTED),
            step("s2", "b", position = 1),
        )
        val a = route(status = DocumentApprovalStatus.REJECTED, steps = steps)

        assertThat(approvalStepState(a, steps[0])).isEqualTo(DocStepState.REJECTED)
        // Nobody's turn: the route is over, and «сейчас его очередь» would be a
        // request for a signature that cannot be given.
        assertThat(approvalStepState(a, steps[1])).isEqualTo(DocStepState.WAITING)
    }

    @Test
    fun `the document's answer is the open route, else the newest closed one`() {
        val open = route(id = "open")
        val closed = route(id = "closed", status = DocumentApprovalStatus.APPROVED)
        val older = route(id = "older", status = DocumentApprovalStatus.REJECTED)

        // Newest first, as the server returns them.
        assertThat(documentApprovalState(listOf(closed, older))?.id).isEqualTo("closed")
        assertThat(documentApprovalState(listOf(closed, open, older))?.id).isEqualTo("open")
        // Never sent for approval reads differently from «not approved».
        assertThat(documentApprovalState(emptyList())).isNull()
    }

    @Test
    fun `one open route per document`() {
        assertThat(canRaiseApproval(emptyList())).isTrue()
        assertThat(canRaiseApproval(listOf(route(status = DocumentApprovalStatus.APPROVED)))).isTrue()
        assertThat(
            canRaiseApproval(listOf(route(status = DocumentApprovalStatus.APPROVED), route())),
        ).isFalse()
    }

    @Test
    fun `links are counted per block, and the unanchored ones are not counted at all`() {
        val links = listOf(
            DocumentTaskLink(id = "l1", blockId = "b1"),
            DocumentTaskLink(id = "l2", blockId = "b1"),
            DocumentTaskLink(id = "l3", blockId = "b2"),
            // Filed against the document as a whole — it belongs to no block, and
            // counting it under one would mark a paragraph nobody linked.
            DocumentTaskLink(id = "l4"),
        )

        assertThat(docLinksByBlock(links)).containsExactly("b1", 2, "b2", 1)
    }
}
