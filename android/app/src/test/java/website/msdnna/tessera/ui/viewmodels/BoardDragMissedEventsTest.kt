package website.msdnna.tessera.ui.viewmodels

import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import io.mockk.coEvery
import io.mockk.coVerify
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
import website.msdnna.tessera.data.realtime.RealtimeEvent
import website.msdnna.tessera.data.repository.BoardRepository

/**
 * Перетаскивание не должно съедать чужие изменения (#2932).
 *
 * Пока карточка в воздухе, доска обязана стоять на месте: SortableJS-аналога тут
 * нет, но перезагрузка списка так же выдернула бы содержимое из-под пальца.
 * Поэтому `dragging` глушит realtime целиком — и до этой правки событие просто
 * выбрасывалось.
 *
 * Отдельная перезагрузка на отпускании нужна ровно потому, что drag, который
 * НИЧЕГО не сдвинул (карточку подняли и вернули, отпустили мимо колонки), не шлёт
 * на сервер ничего и своей перезагрузки не планирует: чужая карточка, созданная за
 * те секунды, не появлялась на экране до следующего события или pull-to-refresh.
 * Веб этот же случай закрывает списком `missedRefetchKinds` + `flushMissed`
 * (`frontend/src/utils/boardEvents.js`, `KanbanBoard.vue`).
 *
 * Тест НЕ зовёт [BoardViewModel.load] — по той же причине, что и
 * `BoardColumnScopeTest`: та читает конфиг вида из DataStore, то есть уходит на
 * настоящий Dispatchers.IO, которого виртуальное время runTest не ждёт. Пара id
 * ставится напрямую (`bindTargetForTest`): проверяется не адрес, а то, теряется
 * событие или догоняется.
 */
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
class BoardDragMissedEventsTest {
    private val dispatcher = StandardTestDispatcher()
    private val repo = mockk<BoardRepository>(relaxUnitFun = true)

    private val todo = BoardColumn(id = "c-todo", name = "К работе", nameKey = "todo")
    private val task = Task(id = "t-1", columnId = "c-todo", title = "чужая карточка")

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        AppContainer.init(ApplicationProvider.getApplicationContext())
        coEvery { repo.columns(any()) } returns listOf(todo)
        coEvery { repo.tasks(any(), any(), any()) } returns listOf(task)
        coEvery { repo.subtasks(any()) } returns emptyList()
        coEvery { repo.board(any()) } returns Board(doneColumnId = null)
    }

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun board() = BoardViewModel(repo).also { it.bindTargetForTest(BOARD, WS) }

    @Test
    fun `событие, пришедшее во время перетаскивания, догоняется на отпускании`() = runTest(dispatcher) {
        val model = board()
        model.dragging = true
        advanceUntilIdle()

        model.onRealtimeEvent(RealtimeEvent(scope = WS, type = "task.created"))
        advanceUntilIdle()
        // Пока палец на экране — ни одного похода в сеть.
        coVerify(exactly = 0) { repo.columns(any()) }

        model.dragging = false
        advanceUntilIdle()

        coVerify(atLeast = 1) { repo.columns(any()) }
        assertThat(model.state.value.tasks.map { it.id }).containsExactly("t-1")
    }

    @Test
    fun `отпускание без пропущенных событий в сеть не ходит`() = runTest(dispatcher) {
        val model = board()
        model.dragging = true
        advanceUntilIdle()
        model.dragging = false
        advanceUntilIdle()

        coVerify(exactly = 0) { repo.columns(any()) }
    }

    @Test
    fun `жест, начатый после приёма события, тоже догоняется`() = runTest(dispatcher) {
        val model = board()
        // Событие принято (drag ещё не начался), но перезагрузка отложена на
        // дебаунс — и палец ложится на экран раньше, чем она успевает уйти.
        model.onRealtimeEvent(RealtimeEvent(scope = WS, type = "task.updated"))
        model.dragging = true
        advanceUntilIdle()
        coVerify(exactly = 0) { repo.columns(any()) }

        model.dragging = false
        advanceUntilIdle()

        coVerify(atLeast = 1) { repo.columns(any()) }
    }

    private companion object {
        const val BOARD = "b-1"
        const val WS = "w-1"
    }
}
