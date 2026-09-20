package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import website.msdnna.tessera.data.model.Relation
import website.msdnna.tessera.data.model.WorkspaceTask

/**
 * Правило подсказки вкладки «Связи» — порт web'ового TaskRelationsTab.vue
 * (linkedNumbers + relGroups).
 */
class RelationPickerTest {

    private fun task(id: String, number: Long?, title: String) =
        WorkspaceTask(id = id, number = number, title = title)

    private fun rel(number: Long, kind: String) =
        Relation(id = "r$number-$kind", kind = kind, relatedNumber = number, relatedTitle = "#$number")

    private val me = "task-self"
    private val candidates = listOf(
        task(me, 1, "Открытая задача"),
        task("t10", 10, "Оплата картой"),
        task("t11", 11, "Оплата счётом"),
        task("t12", 12, "Отчёты"),
        task("t-none", null, "Без номера"),
    )

    private fun numbers(out: List<WorkspaceTask>) = out.mapNotNull { it.number }

    @Test
    fun `уже связанная тем же видом не предлагается`() {
        val out = relationPickerMatches(candidates, me, listOf(rel(10, "relates")), "relates", "оплата")
        assertEquals(listOf(11L), numbers(out))
    }

    @Test
    fun `уже связанная другим видом предлагается`() {
        // Ключ связи — пара + вид: «блокирует #10» не мешает завести «связана с #10».
        val out = relationPickerMatches(candidates, me, listOf(rel(10, "blocks")), "relates", "оплата")
        assertEquals(listOf(10L, 11L), numbers(out))
    }

    @Test
    fun `входящая связь тоже считается связью — список уже в координатах открытой задачи`() {
        // #10 «блокирует» открытую → её строка приходит как blocked_by #10, и выбор
        // blocked_by #10 на сервере не сделал бы ничего.
        val out = relationPickerMatches(candidates, me, listOf(rel(10, "blocked_by")), "blocked_by", "оплата")
        assertEquals(listOf(11L), numbers(out))
    }

    @Test
    fun `поиск по номеру с решёткой находит задачу`() {
        val out = relationPickerMatches(candidates, me, emptyList(), "relates", "#12")
        assertEquals(listOf(12L), numbers(out))
    }

    @Test
    fun `поиск по номеру без решётки находит ту же задачу`() {
        val out = relationPickerMatches(candidates, me, emptyList(), "relates", "12")
        assertEquals(listOf(12L), numbers(out))
    }

    @Test
    fun `открытая задача не предлагается`() {
        val out = relationPickerMatches(candidates, me, emptyList(), "relates", "открытая")
        assertTrue(out.isEmpty())
    }

    @Test
    fun `безномерная задача не предлагается`() {
        // Связь создаётся по номеру — строка без него никуда не ведёт.
        val out = relationPickerMatches(candidates, me, emptyList(), "relates", "без номера")
        assertTrue(out.isEmpty())
    }

    @Test
    fun `пустой запрос — пустой список`() {
        assertTrue(relationPickerMatches(candidates, me, emptyList(), "relates", "   ").isEmpty())
    }

    @Test
    fun `регистр и пробелы вокруг запроса не мешают`() {
        val out = relationPickerMatches(candidates, me, emptyList(), "relates", "  ОПЛАТА  ")
        assertEquals(listOf(10L, 11L), numbers(out))
    }

    @Test
    fun `список режется лимитом`() {
        val many = (1L..40L).map { task("t$it", it, "Задача $it") }
        val out = relationPickerMatches(many, me, emptyList(), "relates", "задача", limit = 5)
        assertEquals(5, out.size)
    }

    @Test
    fun `связи других видов не влияют на набор номеров`() {
        assertEquals(setOf(10L), linkedRelationNumbers(listOf(rel(10, "relates"), rel(11, "blocks")), "relates"))
    }

    @Test
    fun `связь без номера не попадает в набор`() {
        val headless = Relation(id = "r0", kind = "relates", relatedNumber = null)
        assertTrue(linkedRelationNumbers(listOf(headless), "relates").isEmpty())
    }
}
