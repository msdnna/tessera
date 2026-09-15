package website.msdnna.tessera.ui.viewmodels

import org.junit.Assert.assertEquals
import org.junit.Test
import website.msdnna.tessera.data.model.DocumentTaskLink
import website.msdnna.tessera.data.model.WorkspaceTask

/**
 * Кого предлагает пикер задач в панели связей документа (#2932).
 *
 * Связь хранится по ключу (документ, задача, блок) — миграция 0059 заводит его
 * ровно затем, чтобы задача могла быть привязана и к документу целиком, и к
 * пункту, который её порождает. Пикер же отсеивал кандидатов по одному
 * `task_id`, и вторая привязка становилась недостижимой: «Связать блок»
 * открывал список, из которого задача молча пропала. Веб чинится тем же
 * изменением (`frontend/src/components/documents/DocLinks.vue`).
 */
class DocLinkCandidatesTest {

    private val task = WorkspaceTask(id = "t1", title = "Согласовать смету", number = 12)

    private fun state(vararg blocks: String) = DocLinksState(
        links = blocks.map { DocumentTaskLink(id = "l-$it", taskId = "t1", blockId = it) },
        tasks = listOf(task),
    )

    @Test
    fun `задача с привязкой к документу целиком предлагается для блока`() {
        val s = state("").copy(anchorBlockId = "b-7")
        assertEquals(listOf("t1"), s.candidates("").map { it.id })
    }

    @Test
    fun `задача с привязкой к другому блоку предлагается для этого`() {
        val s = state("b-1").copy(anchorBlockId = "b-7")
        assertEquals(listOf("t1"), s.candidates("").map { it.id })
    }

    @Test
    fun `задача с привязкой к тому же блоку не предлагается`() {
        val s = state("b-7").copy(anchorBlockId = "b-7")
        assertEquals(emptyList<String>(), s.candidates("").map { it.id })
    }

    @Test
    fun `задача с привязкой к документу целиком не предлагается без якоря`() {
        assertEquals(emptyList<String>(), state("").candidates("").map { it.id })
    }
}
