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
import website.msdnna.tessera.data.repository.BoardRepository

/**
 * Завершающая колонка доски не должна пропадать из-за моргнувшей сети (#2932).
 *
 * Строку доски перезагрузчики читают best-effort (`runCatching{}.getOrNull()`),
 * а её `done_column_id` клали в состояние безусловно — то есть любое сорвавшееся
 * чтение означало «у доски нет завершающей колонки». Сказать об этом некому:
 * путь нарочно тихий (`silentReload` дёргается на каждом realtime-событии, без
 * всякого действия пользователя). Рядом, строкой выше, `projectId` от того же
 * чтения защищён (`?: projectId`) — асимметрия была прямо в соседних строках.
 *
 * Цена не косметическая: `doneTarget(columns, doneColumnId)` в модалке задачи
 * возвращает null, и «Готово» перестаёт ПЕРЕНОСИТЬ карточку в завершающую
 * колонку — просто штампует `completed` на месте, оставляя её стоять там, где
 * стояла. В шапке колонки заодно слетает глиф и меню предлагает «сделать
 * завершающей» ту, что уже завершающая.
 *
 * Проверяется общий резолвер (`resolveDoneColumn`) через `deleteColumn` —
 * единственный путь с `withBoard = true`, достижимый без [BoardViewModel.load]
 * (та поднимает сокет и читает DataStore; см. BoardColumnScopeTest).
 * `silentReload` считает завершающую тем же резолвером.
 */
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
class BoardDoneColumnTest {
    private val dispatcher = StandardTestDispatcher()
    private val repo = mockk<BoardRepository>(relaxUnitFun = true)

    private val todo = BoardColumn(id = "c-todo", name = "К работе", nameKey = "todo")
    private val done = BoardColumn(id = "c-done", name = "Готово", nameKey = "done")
    private val task = Task(id = "t-1", columnId = "c-todo", title = "задача")

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        AppContainer.init(ApplicationProvider.getApplicationContext())
        coEvery { repo.columns(any()) } returns listOf(todo, done)
        coEvery { repo.tasks(any(), null, false) } returns listOf(task)
        coEvery { repo.subtasks(any()) } returns emptyList()
        coEvery { repo.board(any()) } returns Board(doneColumnId = done.id)
        coEvery { repo.setDoneColumn(any(), any()) } returns Board(doneColumnId = done.id)
    }

    @After
    fun tearDown() = Dispatchers.resetMain()

    /** Сорванное чтение доски — не ответ «завершающей нет». */
    @Test
    fun `упавшее перечитывание доски не снимает завершающую колонку`() = runTest(dispatcher) {
        val model = BoardViewModel(repo)
        model.setDoneColumn(done.id)
        advanceUntilIdle()
        assertThat(model.state.value.doneColumnId).isEqualTo("c-done")

        coEvery { repo.board(any()) } throws IllegalStateException("сеть моргнула")
        model.deleteColumn(todo.id) // сносим ДРУГУЮ колонку — завершающая жива
        advanceUntilIdle()

        assertThat(model.state.value.doneColumnId).isEqualTo("c-done")
    }

    /** Но снесённая завершающая снимается и без успешного чтения доски: её больше
     *  нет в списке колонок, а `boards.done_column_id` объявлен ON DELETE SET NULL. */
    @Test
    fun `снесённая завершающая колонка снимается даже без чтения доски`() = runTest(dispatcher) {
        val model = BoardViewModel(repo)
        model.setDoneColumn(done.id)
        advanceUntilIdle()

        coEvery { repo.board(any()) } throws IllegalStateException("сеть моргнула")
        coEvery { repo.columns(any()) } returns listOf(todo) // сервер её уже удалил
        model.deleteColumn(done.id)
        advanceUntilIdle()

        assertThat(model.state.value.doneColumnId).isNull()
    }

    /** Ответ PATCH'а — это и есть обновлённая доска; лишнего чтения быть не должно. */
    @Test
    fun `установка завершающей колонки берёт ответ записи, а не отдельное чтение`() = runTest(dispatcher) {
        coEvery { repo.board(any()) } throws IllegalStateException("сеть моргнула")
        val model = BoardViewModel(repo)

        model.setDoneColumn(done.id)
        advanceUntilIdle()

        assertThat(model.state.value.doneColumnId).isEqualTo("c-done")
        coVerify(exactly = 0) { repo.board(any()) }
    }
}
