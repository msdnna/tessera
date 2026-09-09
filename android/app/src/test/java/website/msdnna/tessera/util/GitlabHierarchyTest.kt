package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import com.google.gson.Gson
import org.junit.Test
import website.msdnna.tessera.data.model.GitlabIntegration
import website.msdnna.tessera.data.model.GitlabWriteback
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.data.model.TaskDetail

class GitlabHierarchyTest {
    private fun binding(
        boardId: String?,
        enabled: Boolean = true,
        pushChildren: Boolean = true,
        pushCreate: Boolean = false,
    ) = GitlabIntegration(
        id = "i-$boardId",
        boardId = boardId,
        enabled = enabled,
        writeback = GitlabWriteback(pushCreate = pushCreate, pushChildren = pushChildren),
    )

    private fun sub(iid: Long? = null, parentGid: String? = null) =
        Task(id = "s1", glIid = iid, glParentGlobalId = parentGid)

    // ── glSubtaskState ──

    @Test
    fun `a subtask without an issue is absent`() {
        assertThat(glSubtaskState(sub())).isEqualTo(GlSubtaskState.ABSENT)
        // The backend sends null, but a zero iid is not an issue either.
        assertThat(glSubtaskState(sub(iid = 0))).isEqualTo(GlSubtaskState.ABSENT)
    }

    @Test
    fun `an issue with no parent work item is detached, not a child`() {
        assertThat(glSubtaskState(sub(iid = 42))).isEqualTo(GlSubtaskState.DETACHED)
        // GitLab answers an unaccepted hierarchy with an empty string, not with null.
        assertThat(glSubtaskState(sub(iid = 42, parentGid = ""))).isEqualTo(GlSubtaskState.DETACHED)
        assertThat(glSubtaskState(sub(iid = 42, parentGid = "  "))).isEqualTo(GlSubtaskState.DETACHED)
    }

    @Test
    fun `an issue under a parent work item is a child`() {
        val state = glSubtaskState(sub(iid = 42, parentGid = "gid://gitlab/WorkItem/900"))
        assertThat(state).isEqualTo(GlSubtaskState.CHILD)
    }

    // ── gitlabCanGroup ──

    @Test
    fun `grouping needs a binding on this very board`() {
        assertThat(gitlabCanGroup(emptyList(), "board-1")).isFalse()
        assertThat(gitlabCanGroup(listOf(binding("board-2")), "board-1")).isFalse()
        assertThat(gitlabCanGroup(listOf(binding("board-1")), "board-1")).isTrue()
    }

    @Test
    fun `a disabled binding or push_children off means no grouping`() {
        assertThat(gitlabCanGroup(listOf(binding("board-1", enabled = false)), "board-1")).isFalse()
        assertThat(gitlabCanGroup(listOf(binding("board-1", pushChildren = false)), "board-1")).isFalse()
    }

    @Test
    fun `grouping is not a sub-option of issue creation`() {
        // A binding may push children into an existing hierarchy while refusing to
        // create issues from tasks — and the other way round.
        val childrenOnly = binding("board-1", pushChildren = true, pushCreate = false)
        assertThat(gitlabCanGroup(listOf(childrenOnly), "board-1")).isTrue()
        assertThat(gitlabCreateCaps(listOf(childrenOnly), "board-1").canCreate).isFalse()

        val createOnly = binding("board-1", pushChildren = false, pushCreate = true)
        assertThat(gitlabCanGroup(listOf(createOnly), "board-1")).isFalse()
        assertThat(gitlabCreateCaps(listOf(createOnly), "board-1").canCreate).isTrue()
    }

    @Test
    fun `one grouping binding among several is enough`() {
        val all = listOf(binding("board-2"), binding(null), binding("board-1"))
        assertThat(gitlabCanGroup(all, "board-1")).isTrue()
    }

    @Test
    fun `a blank board id resolves to nothing`() {
        assertThat(gitlabCanGroup(listOf(binding(null)), "")).isFalse()
    }

    // ── the wire shapes these read ──

    @Test
    fun `subtask rows carry gl_ names, board cards carry gitlab_ ones`() {
        // One Kotlin class, two backend queries: ListSubtasksWithMeta returns the
        // gitlab_links columns raw, ListBoardTasksWithMeta aliases them. Parsing a
        // subtask row through the board-card names (or the reverse) would silently
        // report every subtask as ABSENT, so both sets are pinned here.
        val gson = Gson()
        val subRow = gson.fromJson(
            """{"id":"s1","gl_iid":7,"gl_web_url":"https://gl/x/-/issues/7",
               "gl_parent_global_id":"gid://gitlab/WorkItem/1"}""",
            Task::class.java,
        )
        assertThat(subRow.glIid).isEqualTo(7)
        assertThat(subRow.glWebUrl).isEqualTo("https://gl/x/-/issues/7")
        assertThat(glSubtaskState(subRow)).isEqualTo(GlSubtaskState.CHILD)

        val card = gson.fromJson("""{"id":"t1","gitlab_iid":7,"gitlab_url":"https://gl"}""", Task::class.java)
        assertThat(card.gitlabIid).isEqualTo(7)
        assertThat(card.glIid).isNull()
    }

    @Test
    fun `is_group rides on the task's gitlab link`() {
        val gson = Gson()
        val grouped = gson.fromJson("""{"id":"t1","gitlab":{"iid":3,"is_group":true}}""", TaskDetail::class.java)
        assertThat(grouped.gitlab?.isGroup).isTrue()
        // Absent flag = not grouped; a linked task from an older backend must not
        // light up the badge.
        val plain = gson.fromJson("""{"id":"t1","gitlab":{"iid":3}}""", TaskDetail::class.java)
        assertThat(plain.gitlab?.isGroup).isFalse()
    }

    @Test
    fun `push_children survives a writeback round-trip`() {
        // The integration editor sends the whole writeback back. Before this field
        // existed, saving a binding from Android cleared what web had configured.
        val gson = Gson()
        val parsed = gson.fromJson(
            """{"enabled":true,"push_create":true,"push_children":true}""",
            GitlabWriteback::class.java,
        )
        assertThat(parsed.pushChildren).isTrue()
        assertThat(gson.toJson(parsed)).contains("\"push_children\":true")
    }
}
