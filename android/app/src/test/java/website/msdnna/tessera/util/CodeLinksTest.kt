package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import com.google.gson.Gson
import org.junit.Test
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.CodeLink
import website.msdnna.tessera.data.model.TaskDetail

/**
 * «Разработка» окна задачи (#2964): что пришло в `code_links` и как это читается
 * на карточке — паритет с вебом (`frontend/src/utils/codeLinks.js`).
 */
class CodeLinksTest {
    @Test
    fun `the task detail carries its merge requests, and none reads as an empty list`() {
        val json = """{"id":"t1","code_links":[{"provider":"gitlab","kind":"merge_request","number":418,
            "title":"Редизайн доски v2","state":"open","draft":true,"source_branch":"feat/board-v2",
            "target_branch":"main","ci_status":"running","url":"https://gl/x/-/merge_requests/418"}]}"""
        val d = Gson().fromJson(json, TaskDetail::class.java)
        assertThat(d.codeLinks).hasSize(1)
        val mr = d.codeLinks.first()
        assertThat(mr.number).isEqualTo(418)
        assertThat(mr.draft).isTrue()
        assertThat(mr.sourceBranch).isEqualTo("feat/board-v2")
        assertThat(Gson().fromJson("""{"id":"t2"}""", TaskDetail::class.java).codeLinks).isEmpty()
    }

    @Test
    fun `the state pill — a draft is neutral, merged and closed win over draft`() {
        assertThat(changeStatus(CodeLink(state = "open"))).isEqualTo(ChangeStatus(R.string.task_dev_state_open, "#18a058"))
        assertThat(changeStatus(CodeLink(state = "open", draft = true)).hex).isNull()
        assertThat(changeStatus(CodeLink(state = "open", draft = true)).label).isEqualTo(R.string.task_dev_state_draft)
        assertThat(changeStatus(CodeLink(state = "merged", draft = true)).label).isEqualTo(R.string.task_dev_state_merged)
        assertThat(changeStatus(CodeLink(state = "closed")).label).isEqualTo(R.string.task_dev_state_closed)
    }

    @Test
    fun `the pipeline line — no pipeline draws nothing`() {
        assertThat(ciStatus("success")?.label).isEqualTo(R.string.task_dev_ci_success)
        assertThat(ciStatus("running")?.label).isEqualTo(R.string.task_dev_ci_running)
        assertThat(ciStatus("failed")?.label).isEqualTo(R.string.task_dev_ci_failed)
        assertThat(ciStatus("canceled")?.label).isEqualTo(R.string.task_dev_ci_canceled)
        assertThat(ciStatus("manual")?.label).isEqualTo(R.string.task_dev_ci_manual)
        assertThat(ciStatus("")).isNull()
        assertThat(ciStatus(null)).isNull()
    }

    @Test
    fun `the branch line`() {
        assertThat(branchLine(CodeLink(sourceBranch = "feat/board-v2", targetBranch = "main"))).isEqualTo("feat/board-v2 → main")
        assertThat(branchLine(CodeLink(sourceBranch = "feat/x"))).isEqualTo("feat/x")
        assertThat(branchLine(CodeLink())).isEmpty()
    }
}
