package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.Conference
import website.msdnna.tessera.data.model.Document
import website.msdnna.tessera.data.model.DocumentTemplate
import website.msdnna.tessera.data.model.WorkspaceMilestone

/**
 * Документы и шаблоны v2 (#3032) на телефоне: чипы этапа и встречи над текстом
 * (#3044, #3046), категории галереи шаблонов (#3045) и колонки шаблонов досок
 * (#3047) — паритет web.
 */
class DocPinsTest {
    private val stages = listOf(WorkspaceMilestone(id = "m1", title = "Спринт 42", projectName = "Tessera Web"))
    private val meetings = listOf(Conference(id = "c1", title = "Планирование Спринта 43"))

    @Test
    fun `chips are captioned from the workspace lists`() {
        val pins = docPins(Document(id = "d", milestoneId = "m1", conferenceId = "c1"), stages, meetings)
        assertThat(pins.stage).isEqualTo("Спринт 42")
        assertThat(pins.stageProject).isEqualTo("Tessera Web")
        assertThat(pins.meeting).isEqualTo("Планирование Спринта 43")
        assertThat(pins.isEmpty).isFalse()
    }

    @Test
    fun `a pin missing from the lists draws no chip`() {
        val pins = docPins(Document(id = "d", milestoneId = "gone"), stages, meetings)
        assertThat(pins.stage).isNull()
        assertThat(pins.isEmpty).isTrue()
        assertThat(docPins(null, stages, meetings).isEmpty).isTrue()
    }

    @Test
    fun `lists are fetched only for a pinned document`() {
        assertThat(docHasPins(Document(id = "d"))).isFalse()
        assertThat(docHasPins(Document(id = "d", conferenceId = "c1"))).isTrue()
        assertThat(docHasPins(null)).isFalse()
    }

    @Test
    fun `gallery sources - categories and my templates`() {
        val mine = templateCard(DocumentTemplate(id = "a", title = "Мой", authorId = "u1", category = "sprint"))
        val theirs = templateCard(DocumentTemplate(id = "b", title = "Чужой", authorId = "u2"))
        val retro = builtinTemplateCard(DocBuiltinTemplate.RETRO, "Ретро", "")
        val cards = listOf(mine, theirs, retro)

        assertThat(filterDocTemplates(cards, "", "all", "u1")).hasSize(3)
        assertThat(filterDocTemplates(cards, "", "mine", "u1").map { it.id }).containsExactly("a")
        assertThat(filterDocTemplates(cards, "", "mine", "")).isEmpty()
        assertThat(filterDocTemplates(cards, "", "sprint", "u1").map { it.id })
            .containsExactly("a", "builtin:retro")
        assertThat(filterDocTemplates(cards, "ретро", "sprint", "u1").map { it.id })
            .containsExactly("builtin:retro")
    }

    @Test
    fun `every built-in is filed under a gallery category`() {
        val categories = DOC_TEMPLATE_SOURCES - setOf("all", "mine")
        DocBuiltinTemplate.entries.forEach { assertThat(categories).contains(it.category) }
        assertThat(DocBuiltinTemplate.MEETING.category).isEqualTo("meetings")
    }

    @Test
    fun `the bug tracker's verify column reads as review`() {
        assertThat(isReviewColumn("verify", "Проверка")).isTrue()
        assertThat(isReviewColumn("confirmed", "Подтверждены")).isFalse()
    }
}
