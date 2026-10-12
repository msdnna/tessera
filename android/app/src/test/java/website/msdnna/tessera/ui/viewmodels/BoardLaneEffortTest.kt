package website.msdnna.tessera.ui.viewmodels

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.util.BoardFilter
import website.msdnna.tessera.util.Estimation

/**
 * «Σ» в шапке дорожки — трудозатраты её карточек (#2932).
 *
 * Дорожка (этап на доске, swimlane таймлайна) держит только ВЕРХНЕУРОВНЕВЫЕ карточки:
 * подзадачи рисуются под родителем. Сама карточка печатает `own ?: свёртка подзадач`
 * («Σ 5д»), а шапка складывала только свои `estimate` строк — и задача, работа которой
 * расписана целиком по подзадачам, давала в итог ноль. По шапке читают загрузку, а
 * выпадали из неё именно декомпозированные, то есть крупные.
 *
 * Дети берутся из [BoardUiState.visibleSubtasksOf] — тех же, из которых нарисована
 * карточка, — поэтому под фильтром шапка сужается вместе с ней.
 */
class BoardLaneEffortTest {
    private val todo = BoardColumn(id = "c-todo", name = "To do", position = 1.0)

    private fun state(
        tasks: List<Task>,
        subtasks: List<Task> = emptyList(),
        filter: BoardFilter = BoardFilter(),
    ) = BoardUiState(
        columns = listOf(todo),
        tasks = tasks,
        subtasks = subtasks,
        filter = filter,
    )

    private fun card(id: String, estimate: Double? = null) =
        Task(id = id, columnId = "c-todo", title = id, estimate = estimate)

    private fun sub(id: String, parent: String, estimate: Double? = null, tags: List<String> = emptyList()) =
        Task(id = id, parentId = parent, columnId = "c-todo", title = id, estimate = estimate, tagIds = tags)

    // ── сама находка ─────────────────────────────────────────────────────────
    @Test
    fun `decomposed card contributes its subtask rollup to the lane total`() {
        val s = state(
            tasks = listOf(card("t-1")),
            subtasks = listOf(sub("s-1", "t-1", 120.0), sub("s-2", "t-1", 60.0)),
        )
        assertThat(s.laneEffort(s.tasks)).isEqualTo(180.0)
    }

    @Test
    fun `lane total equals what the cards under it print`() {
        val own = card("t-1", 90.0)
        val decomposed = card("t-2")
        val s = state(
            tasks = listOf(own, decomposed),
            subtasks = listOf(sub("s-1", "t-2", 30.0), sub("s-2", "t-2", 45.0)),
        )
        // То же правило, что у карточки: своя оценка, а без неё — свёртка детей.
        val printed = s.tasks.map { Estimation.effective(it.estimate, s.visibleSubtasksOf(it.id).map { c -> c.estimate }) }
        assertThat(printed).containsExactly(90.0, 75.0).inOrder()
        assertThat(s.laneEffort(s.tasks)).isEqualTo(165.0)
    }

    @Test
    fun `own estimate wins over the subtask rollup and is not added to it`() {
        val s = state(
            tasks = listOf(card("t-1", 100.0)),
            subtasks = listOf(sub("s-1", "t-1", 60.0), sub("s-2", "t-1", 60.0)),
        )
        assertThat(s.laneEffort(s.tasks)).isEqualTo(100.0)
    }

    @Test
    fun `filter narrows the rollup with the children the card draws`() {
        // Родитель сам под фильтр не подходит — его держит на доске только помеченный
        // ребёнок, и список детей сужается до него же.
        val s = state(
            tasks = listOf(card("t-1")),
            subtasks = listOf(sub("s-1", "t-1", 60.0, tags = listOf("tag-1")), sub("s-2", "t-1", 120.0)),
            filter = BoardFilter(tagIds = setOf("tag-1")),
        )
        assertThat(s.visibleSubtasksOf("t-1").map { it.id }).containsExactly("s-1")
        assertThat(s.laneEffort(s.visibleTasksIn("c-todo"))).isEqualTo(60.0)
    }

    // ── контроли ─────────────────────────────────────────────────────────────
    @Test
    fun `parent matching by itself keeps its whole subtree in the rollup`() {
        val s = state(
            tasks = listOf(card("t-1").copy(tagIds = listOf("tag-1"))),
            subtasks = listOf(sub("s-1", "t-1", 60.0, tags = listOf("tag-1")), sub("s-2", "t-1", 120.0)),
            filter = BoardFilter(tagIds = setOf("tag-1")),
        )
        assertThat(s.laneEffort(s.visibleTasksIn("c-todo"))).isEqualTo(180.0)
    }

    @Test
    fun `nothing estimated anywhere leaves the header empty`() {
        val s = state(tasks = listOf(card("t-1")), subtasks = listOf(sub("s-1", "t-1")))
        assertThat(s.laneEffort(s.tasks)).isNull()
    }

    @Test
    fun `an empty lane has no total`() {
        assertThat(state(tasks = emptyList()).laneEffort(emptyList())).isNull()
    }

    @Test
    fun `only direct children roll up, a grandchild does not`() {
        // Карточка печатает свёртку ПРЯМЫХ детей — шапка обязана читать так же.
        val s = state(
            tasks = listOf(card("t-1")),
            subtasks = listOf(sub("s-1", "t-1"), sub("g-1", "s-1", 240.0)),
        )
        assertThat(s.laneEffort(s.tasks)).isNull()
    }

    @Test
    fun `zero and negative estimates are ignored`() {
        val s = state(
            tasks = listOf(card("t-1", 0.0), card("t-2", -30.0), card("t-3", 45.0)),
        )
        assertThat(s.laneEffort(s.tasks)).isEqualTo(45.0)
    }

    @Test
    fun `a zero own estimate still falls through to the subtask rollup`() {
        // `own ?: rollup` — подменяет только отсутствующее значение; ноль до сложения
        // не доживает (его отсеивает sum), и итог у дорожки пустой.
        val s = state(tasks = listOf(card("t-1", 0.0)), subtasks = listOf(sub("s-1", "t-1", 60.0)))
        assertThat(Estimation.effective(0.0, listOf(60.0))).isEqualTo(0.0)
        assertThat(s.laneEffort(s.tasks)).isNull()
    }

    // ── чистое правило ───────────────────────────────────────────────────────
    @Test
    fun `effective reads a card the way the card prints itself`() {
        assertThat(Estimation.effective(90.0, listOf(30.0, 30.0))).isEqualTo(90.0)
        assertThat(Estimation.effective(null, listOf(30.0, 30.0))).isEqualTo(60.0)
        assertThat(Estimation.effective(null, listOf(null, null))).isNull()
        assertThat(Estimation.effective(null, emptyList())).isNull()
    }

    @Test
    fun `laneEffort sums the effective value of every row`() {
        val rows = listOf(90.0 to listOf<Double?>(), null to listOf<Double?>(30.0, 45.0), null to listOf<Double?>())
        val total = Estimation.laneEffort(rows, { it.first }, { it.second })
        assertThat(total).isEqualTo(165.0)
    }
}
