package website.msdnna.tessera.ui.viewmodels

import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Board
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.data.repository.BoardRepository

/**
 * Правка колонки не должна менять область просмотра доски (#2932).
 *
 * `scopedTasks()` носит в доке обещание «Every reload path goes through here», и
 * ровно один путь его обходил: `reloadColumns()` тянул карточки сырыми
 * `repo.tasks(boardId)` / `repo.subtasks(boardId)`. Через него идут все пять
 * операций над колонкой — переименование, цвет, создание, удаление, перенос.
 *
 * Под спринт-областью это молча расширяло доску до всех карточек, пока чип
 * «Этап» продолжал обещать сужение. В архиве — хуже: `archivedMode` остаётся,
 * чип «Архив» висит, а в дорожках оказываются ЖИВЫЕ карточки, и меню карточки
 * выбирается по флагу области (`TaskCard`: `state.archivedMode`), то есть у
 * живой задачи открывается архивное меню с «Удалить навсегда».
 *
 * Отдельно — удаление колонки: `boards.done_column_id` объявлен ON DELETE SET
 * NULL, так что снос завершающей колонки оставляет доску без неё, а своё же
 * WS-эхо гасит `markLocalChange()`. Веб перечитывает доску на column-purge
 * (`utils/boardEvents.js`), теперь перечитывает и телефон.
 *
 * Тест НЕ зовёт [BoardViewModel.load]: та читает конфиг вида из DataStore, то
 * есть уходит на настоящий Dispatchers.IO, которого виртуальное время runTest не
 * ждёт — загрузка доезжала бы до состояния уже после проверок. Поэтому доска
 * остаётся с дефолтным (пустым) id: все походы в репозиторий замоканы, а
 * проверяется не адрес, а то, КАКУЮ выборку задач выбирает ViewModel.
 */
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
class BoardColumnScopeTest {
    private val dispatcher = StandardTestDispatcher()
    private val repo = mockk<BoardRepository>(relaxUnitFun = true)

    private val todo = BoardColumn(id = "c-todo", name = "К работе", nameKey = "todo")
    private val done = BoardColumn(id = "c-done", name = "Готово", nameKey = "done")

    private val liveTask = Task(id = "t-live", columnId = "c-todo", title = "живая")
    private val archivedTask =
        Task(id = "t-arch", columnId = "c-todo", title = "в архиве", archivedAt = "2026-09-01T00:00:00Z")
    private val sprintTask = Task(id = "t-sprint", columnId = "c-todo", title = "в спринте")

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        AppContainer.init(ApplicationProvider.getApplicationContext())
        coEvery { repo.columns(any()) } returns listOf(todo, done)
        coEvery { repo.subtasks(any()) } returns listOf(liveTask)
        // Три разные выборки задач за одной ручкой: живая доска, спринт, архив.
        coEvery { repo.tasks(any(), null, false) } returns listOf(liveTask)
        coEvery { repo.tasks(any(), SPRINT, false) } returns listOf(sprintTask)
        coEvery { repo.tasks(any(), null, true) } returns listOf(archivedTask)
        coEvery { repo.board(any()) } returns Board(doneColumnId = done.id)
        coEvery { repo.setDoneColumn(any(), any()) } returns Board(doneColumnId = done.id)
        coEvery { repo.renameColumn(any(), any()) } returns todo
        coEvery { repo.setColumnColor(any(), any()) } returns todo
        coEvery { repo.createColumn(any(), any()) } returns todo
        coEvery { repo.moveColumn(any(), any(), any()) } returns todo
    }

    @After
    fun tearDown() = Dispatchers.resetMain()

    @Test
    fun `переименование колонки в архиве не подменяет карточки живыми`() = runTest(dispatcher) {
        val model = BoardViewModel(repo)
        model.enterArchive()
        advanceUntilIdle()
        assertThat(model.state.value.tasks.map { it.id }).containsExactly("t-arch")

        model.renameColumn(todo, "Бэклог")
        advanceUntilIdle()

        val s = model.state.value
        // Область не менялась — значит и набор карточек прежний. Иначе у живой
        // задачи открылось бы архивное меню с «Удалить навсегда».
        assertThat(s.archivedMode).isTrue()
        assertThat(s.tasks.map { it.id }).containsExactly("t-arch")
        // Подзадачи архивируются вместе с родителем, так что в архиве их нет.
        assertThat(s.subtasks).isEmpty()
    }

    @Test
    fun `удаление колонки в архиве не подменяет карточки живыми`() = runTest(dispatcher) {
        val model = BoardViewModel(repo)
        model.enterArchive()
        advanceUntilIdle()

        model.deleteColumn(todo.id)
        advanceUntilIdle()

        assertThat(model.state.value.archivedMode).isTrue()
        assertThat(model.state.value.tasks.map { it.id }).containsExactly("t-arch")
    }

    @Test
    fun `создание колонки под спринт-областью не расширяет доску`() = runTest(dispatcher) {
        val model = BoardViewModel(repo)
        model.setMilestoneScope(SPRINT)
        advanceUntilIdle()
        assertThat(model.state.value.tasks.map { it.id }).containsExactly("t-sprint")

        model.createColumn("Тестирование")
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.milestoneScope).isEqualTo(SPRINT)
        assertThat(s.tasks.map { it.id }).containsExactly("t-sprint")
    }

    @Test
    fun `перенос колонки под спринт-областью не расширяет доску`() = runTest(dispatcher) {
        val model = BoardViewModel(repo)
        model.setMilestoneScope(SPRINT)
        advanceUntilIdle()

        model.moveColumn(done.id, todo.id, null)
        advanceUntilIdle()

        assertThat(model.state.value.tasks.map { it.id }).containsExactly("t-sprint")
    }

    @Test
    fun `снос завершающей колонки оставляет доску без неё, и состояние это знает`() = runTest(dispatcher) {
        val model = BoardViewModel(repo)
        model.setDoneColumn(done.id)
        advanceUntilIdle()
        assertThat(model.state.value.doneColumnId).isEqualTo("c-done")

        // Сервер обнулит done_column_id сам (ON DELETE SET NULL) — доска после
        // удаления отдаёт null.
        coEvery { repo.board(any()) } returns Board(doneColumnId = null)
        model.deleteColumn(done.id)
        advanceUntilIdle()

        assertThat(model.state.value.doneColumnId).isNull()
    }

    private companion object {
        const val SPRINT = "ms-1"
    }
}
