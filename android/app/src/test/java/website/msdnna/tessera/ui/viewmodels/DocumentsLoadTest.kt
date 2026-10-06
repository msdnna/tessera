package website.msdnna.tessera.ui.viewmodels

import com.google.common.truth.Truth.assertThat
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.delay
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.After
import org.junit.Before
import org.junit.Test
import retrofit2.HttpException
import retrofit2.Response
import website.msdnna.tessera.data.model.Document
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.model.WorkspaceTask
import website.msdnna.tessera.data.repository.DocumentRepository
import website.msdnna.tessera.data.repository.TaskRepository
import website.msdnna.tessera.data.repository.WorkspaceRepository

/**
 * Смена пространства в разделе документов (#2932).
 *
 * `viewModel()` в DocumentsScreen привязан к хосту, а не к пространству: раздел
 * остаётся смонтированным, когда пространство под ним переключают, — и всё
 * состояние прошлого (сетка, крошки, открытая читалка, пикеры панели связей)
 * оставалось в нём. Тот же баг, что уже починен в MembersViewModel.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class DocumentsLoadTest {
    private val dispatcher = StandardTestDispatcher()
    private val repo = mockk<DocumentRepository>()
    private val taskRepo = mockk<TaskRepository>()
    private val wsRepo = mockk<WorkspaceRepository>()

    private val docA = Document(id = "d-a", workspaceId = "ws-1", title = "Устав ws-1")
    private val docB = Document(id = "d-b", workspaceId = "ws-2", title = "Устав ws-2")

    private val taskA = WorkspaceTask(id = "t-a", title = "Задача ws-1", number = 1)
    private val taskB = WorkspaceTask(id = "t-b", title = "Задача ws-2", number = 2)

    private val memberA = Member(userId = "u-a", role = "member", email = "a@e.com")
    private val memberB = Member(userId = "u-b", role = "member", email = "b@e.com")

    @Before
    fun setUp() = Dispatchers.setMain(dispatcher)

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun forbidden(): HttpException {
        val raw = okhttp3.Response.Builder()
            .code(403)
            .message("forbidden")
            .protocol(okhttp3.Protocol.HTTP_1_1)
            .request(okhttp3.Request.Builder().url("http://localhost/").build())
            .build()
        val body = """{"error":"forbidden"}""".toResponseBody("application/json".toMediaType())
        return HttpException(Response.error<Any>(body, raw))
    }

    private fun vm() = DocumentsViewModel(repo, taskRepo, wsRepo)

    /** Открывает документ так, как это делает тайл сетки. */
    private fun stubOpen(doc: Document) {
        coEvery { repo.get(doc.id) } returns doc
        coEvery { repo.comments(doc.id) } returns emptyList()
        coEvery { repo.taskLinks(doc.id) } returns emptyList()
        coEvery { repo.approvals(doc.id) } returns emptyList()
    }

    @Test
    fun `упавший список второго пространства не оставляет документы первого`() = runTest(dispatcher) {
        // Ветка отказа возвращается, ничего не записав, — без сброса чужая сетка
        // остаётся под красной плашкой и выдаёт себя за эту.
        coEvery { repo.list("ws-1") } returns listOf(docA)
        coEvery { repo.list("ws-2") } throws forbidden()
        val model = vm()

        model.load("ws-1")
        advanceUntilIdle()
        assertThat(model.state.value.tiles.map { it.id }).containsExactly("d-a")

        model.load("ws-2")
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.docs).isEmpty()
        assertThat(s.tiles).isEmpty()
        assertThat(s.error).isNotNull()
        assertThat(s.loading).isFalse()
    }

    @Test
    fun `открытая читалка не переезжает в другое пространство`() = runTest(dispatcher) {
        coEvery { repo.list("ws-1") } returns listOf(docA)
        coEvery { repo.list("ws-2") } returns listOf(docB)
        stubOpen(docA)
        val model = vm()

        model.load("ws-1")
        advanceUntilIdle()
        model.open(docA)
        advanceUntilIdle()
        assertThat(model.state.value.openId).isEqualTo("d-a")

        model.load("ws-2")
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.openId).isNull()
        assertThat(s.open).isNull()
        assertThat(s.blocks).isEmpty()
        assertThat(s.tiles.map { it.id }).containsExactly("d-b")
    }

    @Test
    fun `пикеры панели связей не предлагают задач и участников чужого пространства`() = runTest(dispatcher) {
        // Пикеры кэшируются «однажды и навсегда» (задачи пространства — не то,
        // что тянут на каждый открытый документ), и кэш переживал переключение:
        // панель предлагала связать документ с задачей чужого пространства, на
        // что бэкенд честно отвечает 400.
        coEvery { repo.list("ws-1") } returns listOf(docA)
        coEvery { repo.list("ws-2") } returns listOf(docB)
        stubOpen(docA)
        stubOpen(docB)
        coEvery { taskRepo.workspaceTasks("ws-1") } returns listOf(taskA)
        coEvery { taskRepo.workspaceTasks("ws-2") } returns listOf(taskB)
        coEvery { wsRepo.members("ws-1") } returns listOf(memberA)
        coEvery { wsRepo.members("ws-2") } returns listOf(memberB)
        val model = vm()

        model.load("ws-1")
        advanceUntilIdle()
        model.open(docA)
        advanceUntilIdle()
        model.openLinks()
        advanceUntilIdle()
        assertThat(model.state.value.links.tasks.map { it.id }).containsExactly("t-a")
        assertThat(model.state.value.links.members.map { it.userId }).containsExactly("u-a")

        model.load("ws-2")
        advanceUntilIdle()
        assertThat(model.state.value.links.tasks).isEmpty()
        assertThat(model.state.value.links.members).isEmpty()

        // И снова наполняются — уже своими.
        model.open(docB)
        advanceUntilIdle()
        model.openLinks()
        advanceUntilIdle()
        assertThat(model.state.value.links.tasks.map { it.id }).containsExactly("t-b")
        assertThat(model.state.value.links.members.map { it.userId }).containsExactly("u-b")
    }

    @Test
    fun `поздний список прошлого пространства не дописывается в состояние`() = runTest(dispatcher) {
        // Старый запрос живёт в viewModelScope, который перезапуск композиции не
        // гасит: два списка бывают в полёте разом.
        coEvery { repo.list("ws-1") } coAnswers {
            delay(50)
            listOf(docA)
        }
        coEvery { repo.list("ws-2") } returns listOf(docB)
        val model = vm()

        model.load("ws-1")
        advanceTimeBy(10)
        model.load("ws-2")
        advanceUntilIdle()

        assertThat(model.state.value.tiles.map { it.id }).containsExactly("d-b")
    }

    @Test
    fun `reload не закрывает читалку`() = runTest(dispatcher) {
        // Контроль: сброс привязан к смене id, а reload приходит с тем же — иначе
        // обновление списка выбрасывало бы читателя из документа.
        coEvery { repo.list("ws-1") } returns listOf(docA)
        stubOpen(docA)
        val model = vm()

        model.load("ws-1")
        advanceUntilIdle()
        model.open(docA)
        advanceUntilIdle()

        model.reload()
        advanceUntilIdle()

        assertThat(model.state.value.openId).isEqualTo("d-a")
        assertThat(model.state.value.open?.id).isEqualTo("d-a")
    }
}
