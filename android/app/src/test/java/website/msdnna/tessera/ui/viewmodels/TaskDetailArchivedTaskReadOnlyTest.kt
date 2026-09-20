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
import website.msdnna.tessera.data.model.TaskDetail
import website.msdnna.tessera.data.repository.BoardRepository
import website.msdnna.tessera.data.repository.DocumentRepository
import website.msdnna.tessera.data.repository.TaskRepository

/**
 * Архивная задача, открытая МИМО архивной области доски, тоже «только смотреть»
 * (#2932).
 *
 * `TaskDetailReadOnlyTest` закрыл соседний случай: карточка, открытая ИЗ архива
 * доски. Флаг туда приходит из `BoardUiState.archivedMode`, то есть это ответ
 * про ЭКРАН, а не про задачу — а половина дверей в модалку идёт мимо экрана:
 * связь на вкладке «Связи» (`currentId = it`), чип `#N` в описании
 * (`resolveTaskNumber`), уведомление из колокольчика (`ListNotifications` архив
 * не фильтрует), ссылка из документа (`ListDocumentTaskLinks` тоже),
 * push/reminder deep-link (`MainActivity` → `openTask`). Все они открывают
 * задачу на ЖИВОЙ доске, где `archivedMode = false`. Гварда на бэкенде нет —
 * архив клиентская конвенция, — так что правки архивной задачи уходили молча и
 * успешно.
 *
 * Поэтому read-only теперь = скоуп ИЛИ собственный `archived_at` задачи. Контроли
 * ниже стерегут обе половины: первая не должна была потеряться, вторая не должна
 * запирать живую задачу.
 */
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
class TaskDetailArchivedTaskReadOnlyTest {
    private val dispatcher = StandardTestDispatcher()
    private val taskRepo = mockk<TaskRepository>(relaxed = true)
    private val boardRepo = mockk<BoardRepository>(relaxed = true)
    private val docRepo = mockk<DocumentRepository>(relaxed = true)

    private val live = TaskDetail(id = "t1", boardId = "b1", columnId = "c1", title = "живая")
    private val archived = live.copy(id = "t2", title = "в архиве", archivedAt = "2026-09-01T10:00:00Z")

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        AppContainer.init(ApplicationProvider.getApplicationContext())
    }

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun vm(detail: TaskDetail, scopeReadOnly: Boolean): TaskDetailViewModel {
        coEvery { taskRepo.detail(any()) } returns detail
        return TaskDetailViewModel(taskRepo, boardRepo, docRepo).also {
            it.load(detail.id, "ws1", "p1", readOnly = scopeReadOnly)
        }
    }

    @Test
    fun `архивная задача с живой доски заперта`() = runTest(dispatcher) {
        val m = vm(archived, scopeReadOnly = false)
        advanceUntilIdle()

        assertThat(m.state.value.detail?.isArchived).isTrue()
        assertThat(m.state.value.readOnly).isTrue()
    }

    @Test
    fun `правки архивной задачи с живой доски не уходят в репозиторий`() = runTest(dispatcher) {
        val m = vm(archived, scopeReadOnly = false)
        advanceUntilIdle()

        m.saveCore("другое имя", "другое описание")
        m.setPriority(3)
        m.setCompleted(true)
        m.moveToColumn("c2", website.msdnna.tessera.util.MoveNeighbors())
        m.postComment("привет", emptyList())
        m.addRelation(42, "relates")
        m.uploadAttachment(byteArrayOf(1, 2, 3), "file.bin", null)
        m.removeAttachment("att1")
        advanceUntilIdle()

        coVerify(exactly = 0) { boardRepo.updateTask(any(), any(), any(), any(), any(), any(), any(), any(), any()) }
        coVerify(exactly = 0) { boardRepo.moveTask(any(), any(), any(), any()) }
        coVerify(exactly = 0) { taskRepo.addComment(any(), any(), any(), any()) }
        coVerify(exactly = 0) { taskRepo.addRelation(any(), any(), any()) }
        coVerify(exactly = 0) { taskRepo.uploadAttachment(any(), any(), any(), any()) }
        coVerify(exactly = 0) { taskRepo.deleteAttachment(any()) }
        assertThat(m.state.value.changed).isFalse()
    }

    @Test
    fun `картинка из редактора в архивную задачу не заливается`() = runTest(dispatcher) {
        val m = vm(archived, scopeReadOnly = false)
        advanceUntilIdle()

        assertThat(m.uploadMediaUrl(byteArrayOf(1), "pic.png", "image/png")).isNull()
        coVerify(exactly = 0) { taskRepo.uploadMedia(any(), any(), any()) }
    }

    @Test
    fun `переход по связи на живую задачу снова открывает правки`() = runTest(dispatcher) {
        // Из архивной задачи на вкладке «Связи» уходят к живой: модалка зовёт
        // load() заново с флагом ЭКРАНА. Он всё так же false — значит запертость
        // не должна «залипнуть» на новой задаче, иначе прошлое чтение архива
        // блокировало бы работу до закрытия модалки.
        val m = vm(archived, scopeReadOnly = false)
        advanceUntilIdle()
        assertThat(m.state.value.readOnly).isTrue()

        coEvery { taskRepo.detail(any()) } returns live
        m.load(live.id, "ws1", "p1", readOnly = false)
        advanceUntilIdle()

        assertThat(m.state.value.readOnly).isFalse()
        m.postComment("привет", emptyList())
        advanceUntilIdle()
        coVerify(exactly = 1) { taskRepo.addComment(any(), any(), any(), any()) }
    }

    @Test
    fun `живая задача в архивной области доски остаётся запертой`() = runTest(dispatcher) {
        // Контроль: вторая половина условия не съела первую.
        val m = vm(live, scopeReadOnly = true)
        advanceUntilIdle()

        assertThat(m.state.value.detail?.isArchived).isFalse()
        assertThat(m.state.value.readOnly).isTrue()
    }

    @Test
    fun `живая задача с живой доски по-прежнему правится`() = runTest(dispatcher) {
        // Контроль: гвард обязан ловить ровно архив и ничего больше.
        val m = vm(live, scopeReadOnly = false)
        advanceUntilIdle()

        assertThat(m.state.value.readOnly).isFalse()
        m.saveDescription("другое описание")
        m.postComment("привет", emptyList())
        advanceUntilIdle()

        coVerify(exactly = 1) { boardRepo.updateTask(any(), any(), any(), any(), any(), any(), any(), any(), any()) }
        coVerify(exactly = 1) { taskRepo.addComment(any(), any(), any(), any()) }
    }
}
