package website.msdnna.tessera.ui.viewmodels

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.Milestone
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.util.BoardFilter

/**
 * Набор дорожек, нарисованных ПУСТЫМИ (#2932).
 *
 * Тумблер «сворачивать пустые колонки» обязан побеждать протухший явный override:
 * колонку, развёрнутую руками, включение правила должно свернуть — иначе тумблер
 * читается как «ничего не делает». Веб чистит overrides по `displayColumns` со
 * счётом из `lists[key]`, то есть по дорожкам активной группировки и ПОСЛЕ фильтра.
 *
 * Android считал этот набор в панели настроек как `columns` + `tasksIn` — а `tasksIn`
 * по своему же комменту «NOT filtered (DnD needs every card)». Расхождение с правилом
 * [BoardUiState.isLaneCollapsed], которому скармливают фильтрованный `lane.tasks.size`,
 * давало две дыры: колонка, опустошённая фильтром, рисовалась пустой, но в «пустые» не
 * попадала; а при группировке по тегам и этапам не чистилось вообще ничего.
 */
class BoardEmptyLaneIdsTest {
    private val todo = BoardColumn(id = "c-todo", name = "To do", position = 1.0)
    private val doing = BoardColumn(id = "c-doing", name = "Doing", position = 2.0)

    private val tagged = Task(id = "t-1", columnId = "c-todo", title = "tagged", tagIds = listOf("tag-1"))
    private val plain = Task(id = "t-2", columnId = "c-doing", title = "plain")

    private val byTag = BoardFilter(tagIds = setOf("tag-1"))

    private fun state(
        groupMode: String = "status",
        filter: BoardFilter = BoardFilter(),
        tagPrefix: String = "",
    ) = BoardUiState(
        columns = listOf(todo, doing),
        tasks = listOf(tagged, plain),
        tags = mapOf(
            "tag-1" to Tag(id = "tag-1", name = "S: bug"),
            "tag-2" to Tag(id = "tag-2", name = "chore"),
        ),
        milestones = listOf(
            Milestone(id = "m-1", title = "Sprint 1", position = 1.0),
            Milestone(id = "m-2", title = "Sprint 2", position = 2.0),
        ),
        groupMode = groupMode,
        tagPrefix = tagPrefix,
        filter = filter,
    )

    @Test
    fun `without a filter only genuinely empty columns count`() {
        assertThat(state().emptyLaneIds).isEmpty()
    }

    @Test
    fun `a column emptied by the filter counts as empty`() {
        // Фильтр по tag-1 оставляет на доске только tagged (колонка c-todo);
        // c-doing нарисована пустой, хотя карточка в ней есть.
        assertThat(state(filter = byTag).emptyLaneIds).containsExactly("c-doing")
    }

    @Test
    fun `an empty column stays empty under a filter that matches nothing`() {
        val ids = state(filter = BoardFilter(tagIds = setOf("tag-missing"))).emptyLaneIds
        assertThat(ids).containsExactly("c-todo", "c-doing")
    }

    @Test
    fun `tag grouping counts tag lanes, not status columns`() {
        // tag-1 держит карточку, tag-2 пуст, «Без тега» собирает plain.
        assertThat(state(groupMode = "tag").emptyLaneIds).containsExactly("tag-2")
    }

    @Test
    fun `namespace mode only counts lanes the prefix leaves`() {
        // Колонками становятся только теги с префиксом «S: » — tag-2 дорожкой не
        // является вовсе, а «Без тега» собирает всё без тега ИЗ ЭТОГО пространства,
        // то есть plain, и пустой не считается.
        assertThat(state(groupMode = "tag", tagPrefix = "S: ").emptyLaneIds).isEmpty()
    }

    @Test
    fun `milestone grouping counts milestone lanes including the none lane`() {
        // Ни у одной карточки этапа нет: обе дорожки этапов пусты, «Без этапа» — нет.
        assertThat(state(groupMode = "milestone").emptyLaneIds).containsExactly("m-1", "m-2")
    }

    @Test
    fun `the none lane is reported when nothing falls outside`() {
        val allTagged = state(groupMode = "tag").copy(
            tasks = listOf(tagged, plain.copy(tagIds = listOf("tag-2"))),
        )
        assertThat(allTagged.emptyLaneIds).containsExactly(BoardUiState.NONE_LANE_ID)
    }

    @Test
    fun `lane buckets carry the same cards the lanes draw`() {
        // Дорожки экрана берут карточки отсюда же — состав дорожки и набор пустых
        // не могут разойтись, только вместе.
        val buckets = state(filter = byTag).laneBuckets.toMap()
        assertThat(buckets.keys).containsExactly("c-todo", "c-doing")
        assertThat(buckets.getValue("c-todo").map { it.id }).containsExactly("t-1")
        assertThat(buckets.getValue("c-doing")).isEmpty()
    }
}
