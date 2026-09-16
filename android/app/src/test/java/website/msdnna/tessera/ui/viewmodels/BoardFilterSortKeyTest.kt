package website.msdnna.tessera.ui.viewmodels

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.GitlabMember
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.util.BoardFilter
import website.msdnna.tessera.util.DateFormatPrefs

/**
 * Ключ кэша отфильтрованных дорожек (#2932).
 *
 * Канбан, календарь, таймлайн и Гант memo'ят результат [BoardUiState.applyFilterSort]:
 * фильтрация с сортировкой — O(n log n) на дорожку, и без кэша доска пересобиралась бы
 * на каждом кадре перетаскивания. Ключом были `(tasks, filter, sortLevels)`, а функция
 * читает больше: через `filteredBoard` — ПОДЗАДАЧИ (родитель остаётся на доске из-за
 * подходящего ребёнка), календарь `dateFormat` и GitLab-участников; через сортировку
 * «по статусу» — позиции колонок.
 *
 * Списки сравниваются структурно, поэтому недобор ключей — не теория: перезагрузка
 * кладёт равный по содержимому `tasks`, и кэш не сбрасывается вовсе. Карточка,
 * которую подняла подзадача, оставалась на доске после снятия тега с подзадачи и не
 * появлялась после навешивания — до первого чужого события.
 *
 * Тест держит обе стороны: ключ ОБЯЗАН меняться, когда меняется ответ
 * `applyFilterSort`, и обязан совпадать на равных состояниях — иначе кэш перестанет
 * быть кэшем.
 */
class BoardFilterSortKeyTest {
    private val todo = BoardColumn(id = "c-todo", name = "To do", position = 1.0)
    private val doing = BoardColumn(id = "c-doing", name = "Doing", position = 2.0)

    private val parent = Task(id = "t-1", columnId = "c-todo", title = "parent")
    private val child = Task(id = "s-1", parentId = "t-1", columnId = "c-todo", title = "child")

    /** Фильтр по тегу: поднимать родителя ребёнком умеет только активный фильтр. */
    private val byTag = BoardFilter(tagIds = setOf("tag-1"))

    private fun state(subtasks: List<Task>) = BoardUiState(
        columns = listOf(todo, doing),
        tasks = listOf(parent),
        subtasks = subtasks,
        filter = byTag,
    )

    /** Прежний (недостаточный) ключ — чтобы тест показывал, что именно он проглатывал. */
    private fun oldKey(s: BoardUiState) = listOf(s.tasks, s.filter, s.sortLevels)

    @Test
    fun `tag removed from a subtask drops the lifted parent and changes the key`() {
        val lifted = state(listOf(child.copy(tagIds = listOf("tag-1"))))
        val dropped = state(listOf(child))

        // Ответ экрана меняется: карточка держалась на доске только ребёнком.
        assertThat(lifted.visibleTasksIn("c-todo").map { it.id }).containsExactly("t-1")
        assertThat(dropped.visibleTasksIn("c-todo")).isEmpty()

        // Карточки при этом те же — старый ключ сброса кэша не давал.
        assertThat(dropped.tasks).isEqualTo(lifted.tasks)
        assertThat(oldKey(dropped)).isEqualTo(oldKey(lifted))

        assertThat(dropped.filterSortKey).isNotEqualTo(lifted.filterSortKey)
    }

    @Test
    fun `tag added to a subtask lifts the parent and changes the key`() {
        val hidden = state(listOf(child))
        val lifted = state(listOf(child.copy(tagIds = listOf("tag-1"))))

        assertThat(hidden.visibleTasksIn("c-todo")).isEmpty()
        assertThat(lifted.visibleTasksIn("c-todo").map { it.id }).containsExactly("t-1")
        assertThat(lifted.filterSortKey).isNotEqualTo(hidden.filterSortKey)
    }

    /** Календарь пользователя: «Сегодня»/«Просрочено» считаются в его зоне. */
    @Test
    fun `timezone change is part of the key`() {
        val base = state(listOf(child))
        val moved = base.copy(dateFormat = DateFormatPrefs(zoneId = "Asia/Tokyo"))
        assertThat(moved.filterSortKey).isNotEqualTo(base.filterSortKey)
    }

    /** Автор GitLab-карточки сопоставляется с человеком через эту таблицу. */
    @Test
    fun `gitlab members are part of the key`() {
        val base = state(listOf(child))
        val linked = base.copy(
            gitlabMembers = listOf(GitlabMember(glUsername = "vasya", tesseraUserId = "u-1")),
        )
        assertThat(linked.filterSortKey).isNotEqualTo(base.filterSortKey)
    }

    /** Сортировка «по статусу» берёт позиции колонок — их перестановка меняет порядок. */
    @Test
    fun `column positions are part of the key`() {
        val base = state(listOf(child))
        val reordered = base.copy(columns = listOf(todo.copy(position = 3.0), doing))
        assertThat(reordered.filterSortKey).isNotEqualTo(base.filterSortKey)
    }

    @Test
    fun `equal states share a key so the cache still holds`() {
        assertThat(state(listOf(child)).filterSortKey).isEqualTo(state(listOf(child)).filterSortKey)
    }
}
