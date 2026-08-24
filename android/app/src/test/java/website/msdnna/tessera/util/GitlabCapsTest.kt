package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.GitlabIntegration
import website.msdnna.tessera.data.model.GitlabWriteback

class GitlabCapsTest {
    private fun binding(
        id: String,
        boardId: String?,
        enabled: Boolean = true,
        pushCreate: Boolean = true,
        fetchTemplates: Boolean = false,
    ) = GitlabIntegration(
        id = id,
        boardId = boardId,
        enabled = enabled,
        writeback = GitlabWriteback(pushCreate = pushCreate, fetchTemplates = fetchTemplates),
    )

    @Test
    fun `no integrations means no creation`() {
        val caps = gitlabCreateCaps(emptyList(), "board-1")
        assertThat(caps.canCreate).isFalse()
        assertThat(caps.fetchTemplates).isFalse()
        assertThat(caps.integrationId).isNull()
    }

    @Test
    fun `a binding on another board does not enable this one`() {
        val caps = gitlabCreateCaps(listOf(binding("i1", "board-2")), "board-1")
        assertThat(caps.canCreate).isFalse()
    }

    @Test
    fun `a disabled binding does not enable creation`() {
        val caps = gitlabCreateCaps(listOf(binding("i1", "board-1", enabled = false)), "board-1")
        assertThat(caps.canCreate).isFalse()
    }

    @Test
    fun `push_create off means no creation`() {
        val caps = gitlabCreateCaps(listOf(binding("i1", "board-1", pushCreate = false)), "board-1")
        assertThat(caps.canCreate).isFalse()
    }

    @Test
    fun `an enabled binding with push_create enables creation`() {
        val caps = gitlabCreateCaps(listOf(binding("i1", "board-1")), "board-1")
        assertThat(caps.canCreate).isTrue()
        assertThat(caps.integrationId).isEqualTo("i1")
        // Templates are opt-in on top of creation, not implied by it.
        assertThat(caps.fetchTemplates).isFalse()
    }

    @Test
    fun `fetch_templates rides on push_create`() {
        val on = gitlabCreateCaps(listOf(binding("i1", "board-1", fetchTemplates = true)), "board-1")
        assertThat(on.fetchTemplates).isTrue()

        // Templates without creation are meaningless — a prefilled description nobody
        // can push. Web gates them the same way.
        val orphan = gitlabCreateCaps(
            listOf(binding("i1", "board-1", pushCreate = false, fetchTemplates = true)),
            "board-1",
        )
        assertThat(orphan.canCreate).isFalse()
        assertThat(orphan.fetchTemplates).isFalse()
    }

    @Test
    fun `the binding of this board wins among several`() {
        val caps = gitlabCreateCaps(
            listOf(
                binding("other", "board-2", fetchTemplates = true),
                binding("mine", "board-1", fetchTemplates = true),
                binding("stale", null),
            ),
            "board-1",
        )
        assertThat(caps.integrationId).isEqualTo("mine")
        assertThat(caps.fetchTemplates).isTrue()
    }

    @Test
    fun `a blank board id resolves to nothing`() {
        assertThat(gitlabCreateCaps(listOf(binding("i1", null)), "").canCreate).isFalse()
    }
}
