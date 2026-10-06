package website.msdnna.tessera.ui.viewmodels

import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.io.File
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
 * Задача, открытая из архива доски, не должна править задачу (#2932).
 *
 * Веб закрыл это четырьмя коммитами: архивная область — «только смотреть», и
 * модалка над ней получает `:readonly="archivedMode"`. На android портирована
 * была лишь доска — drag, «+ задача», пилюли карточки (в `TaskCard` даже
 * написано «tapping … just opens the (read) modal»), — а `BoardScreen` звал
 * `TaskModal` вообще без такого флага: тапом по архивной карточке правились
 * заголовок и описание, переносилась колонка, слались и удалялись комментарии,
 * заливались и удалялись вложения, снимались связи и документы. Гварда на
 * бэкенде нет (архив — клиентская конвенция), так что уходило всё молча и
 * успешно — на мягко удалённой задаче, которой на живой доске нет.
 *
 * Здесь проверяется ВТОРОЙ слой — воронка [TaskDetailViewModel] `mutate`:
 * кнопок в архиве не рисуется, но если до вызова всё же дошли, в репозиторий он
 * не уходит. Первый слой (кнопки) — в `TaskArchiveReadOnlyUiTest`.
 *
 * Скачивание вложения — единственное исключение: оно ничего не меняет на
 * сервере, а архивную задачу чаще всего затем и открывают.
 */
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
class TaskDetailReadOnlyTest {
    private val dispatcher = StandardTestDispatcher()
    private val taskRepo = mockk<TaskRepository>(relaxed = true)
    private val boardRepo = mockk<BoardRepository>(relaxed = true)
    private val docRepo = mockk<DocumentRepository>(relaxed = true)

    private val detail = TaskDetail(id = "t1", boardId = "b1", columnId = "c1", title = "в архиве")

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        AppContainer.init(ApplicationProvider.getApplicationContext())
        coEvery { taskRepo.detail(any()) } returns detail
    }

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun vm(readOnly: Boolean) = TaskDetailViewModel(taskRepo, boardRepo, docRepo).also {
        it.load("t1", "ws1", "p1", readOnly = readOnly)
    }

    @Test
    fun `флаг режима доезжает до состояния`() = runTest(dispatcher) {
        val open = vm(readOnly = false)
        advanceUntilIdle()
        assertThat(open.state.value.readOnly).isFalse()

        val archived = vm(readOnly = true)
        advanceUntilIdle()
        assertThat(archived.state.value.readOnly).isTrue()
    }

    @Test
    fun `в архиве ни одна правка не уходит в репозиторий`() = runTest(dispatcher) {
        val m = vm(readOnly = true)
        advanceUntilIdle()

        m.saveCore("другое имя", "другое описание")
        m.saveDescription("другое описание")
        m.setPriority(3)
        m.setCompleted(true)
        m.moveToColumn("c2", website.msdnna.tessera.util.MoveNeighbors())
        m.toggleTag("tag1")
        m.toggleAssignee("u1")
        m.addSubtask("c1", "подзадача")
        m.postComment("привет", emptyList())
        m.deleteComment("cm1")
        m.addRelation(42, "relates")
        m.removeRelation("rel1")
        m.unlinkDocument("link1")
        m.uploadAttachment(byteArrayOf(1, 2, 3), "file.bin", "application/octet-stream")
        m.removeAttachment("att1")
        m.transfer("b2") {}
        m.archive {}
        m.delete {}
        advanceUntilIdle()

        coVerify(exactly = 0) { boardRepo.updateTask(any(), any(), any(), any(), any(), any(), any(), any(), any()) }
        coVerify(exactly = 0) { boardRepo.moveTask(any(), any(), any(), any()) }
        coVerify(exactly = 0) { boardRepo.createTask(any(), any(), any(), any()) }
        coVerify(exactly = 0) { boardRepo.archiveTask(any()) }
        coVerify(exactly = 0) { boardRepo.deleteTask(any()) }
        coVerify(exactly = 0) { taskRepo.addComment(any(), any(), any(), any()) }
        coVerify(exactly = 0) { taskRepo.deleteComment(any()) }
        coVerify(exactly = 0) { taskRepo.addRelation(any(), any(), any()) }
        coVerify(exactly = 0) { taskRepo.deleteRelation(any()) }
        coVerify(exactly = 0) { taskRepo.uploadAttachment(any(), any(), any(), any()) }
        coVerify(exactly = 0) { taskRepo.deleteAttachment(any()) }
        coVerify(exactly = 0) { taskRepo.transfer(any(), any(), any()) }
        coVerify(exactly = 0) { docRepo.unlinkTask(any()) }
        // И ничего не «изменилось»: иначе закрытие модалки перечитывало бы доску,
        // объявляя правку, которой не было.
        assertThat(m.state.value.changed).isFalse()
    }

    @Test
    fun `картинка из редактора в архиве не заливается`() = runTest(dispatcher) {
        val m = vm(readOnly = true)
        advanceUntilIdle()

        // Единственная правка, которая не идёт через mutate: composable зовёт её
        // напрямую и ждёт URL. Файл без текста, который на него ссылается, — это
        // мусор на диске, которого потом никто не найдёт.
        val url = m.uploadMediaUrl(byteArrayOf(1), "pic.png", "image/png")

        assertThat(url).isNull()
        coVerify(exactly = 0) { taskRepo.uploadMedia(any(), any(), any()) }
    }

    @Test
    fun `вложение из архива всё равно скачивается`() = runTest(dispatcher) {
        val file = File.createTempFile("att", ".bin")
        coEvery { taskRepo.downloadAttachment(any(), any(), any()) } returns file
        val m = vm(readOnly = true)
        advanceUntilIdle()

        var got: File? = null
        m.downloadAttachment(file.parentFile!!, "att1", "att.bin") { got = it }
        advanceUntilIdle()

        assertThat(got).isEqualTo(file)
        coVerify(exactly = 1) { taskRepo.downloadAttachment(any(), "att1", "att.bin") }
    }

    @Test
    fun `на живой задаче правки по-прежнему уходят`() = runTest(dispatcher) {
        // Контроль: гвард обязан ловить ровно архив и ничего больше.
        val m = vm(readOnly = false)
        advanceUntilIdle()

        m.saveDescription("другое описание")
        m.postComment("привет", emptyList())
        m.uploadAttachment(byteArrayOf(1, 2, 3), "file.bin", null)
        m.removeAttachment("att1")
        m.deleteComment("cm1")
        advanceUntilIdle()

        coVerify(exactly = 1) { boardRepo.updateTask(any(), any(), any(), any(), any(), any(), any(), any(), any()) }
        coVerify(exactly = 1) { taskRepo.addComment(any(), any(), any(), any()) }
        coVerify(exactly = 1) { taskRepo.uploadAttachment(any(), any(), any(), any()) }
        coVerify(exactly = 1) { taskRepo.deleteAttachment("att1") }
        coVerify(exactly = 1) { taskRepo.deleteComment("cm1") }
    }
}
