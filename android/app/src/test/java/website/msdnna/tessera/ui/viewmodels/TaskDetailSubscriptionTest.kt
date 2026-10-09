package website.msdnna.tessera.ui.viewmodels

import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import com.google.gson.Gson
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
import website.msdnna.tessera.data.model.AssigneeUser
import website.msdnna.tessera.data.model.TaskDetail
import website.msdnna.tessera.data.repository.BoardRepository
import website.msdnna.tessera.data.repository.DocumentRepository
import website.msdnna.tessera.data.repository.TaskRepository

/**
 * «Подписаны / Отписаться» (#2963): кнопка шлёт обратное текущему, список
 * перечитывается с деталью, а архивную задачу (только чтение) читать — значит и
 * следить за ней: подписка её не меняет и через read-only гейт не идёт.
 */
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
class TaskDetailSubscriptionTest {
    private val dispatcher = StandardTestDispatcher()
    private val taskRepo = mockk<TaskRepository>(relaxed = true)
    private val boardRepo = mockk<BoardRepository>(relaxed = true)
    private val docRepo = mockk<DocumentRepository>(relaxed = true)

    private val me = AssigneeUser(id = "u1", email = "me@x", name = "Я")

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        AppContainer.init(ApplicationProvider.getApplicationContext())
    }

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun detail(json: String): TaskDetail = Gson().fromJson(json, TaskDetail::class.java)

    @Test
    fun `subscribers null с сервера — пустой список`() {
        val d = detail("""{"id":"t1","subscribers":null}""")
        assertThat(d.subscribers).isEmpty()
        assertThat(d.subscribed).isFalse()
    }

    @Test
    fun `отписка шлёт false и перечитывает список`() = runTest(dispatcher) {
        val before = detail("""{"id":"t1","subscribed":true,"subscribers":[{"id":"u1","name":"Я"}]}""")
        val after = detail("""{"id":"t1","subscribed":false,"subscribers":[]}""")
        coEvery { taskRepo.detail("t1") } returnsMany listOf(before, after)
        val m = TaskDetailViewModel(taskRepo, boardRepo, docRepo).also { it.load("t1", "ws1", "p1") }
        advanceUntilIdle()
        assertThat(m.state.value.detail?.subscribers?.map { it.id }).containsExactly(me.id)

        m.toggleSubscription()
        advanceUntilIdle()

        coVerify(exactly = 1) { taskRepo.setSubscription("t1", false) }
        assertThat(m.state.value.detail?.subscribed).isFalse()
        assertThat(m.state.value.detail?.subscribers).isEmpty()
        assertThat(m.state.value.changed).isFalse()
    }

    @Test
    fun `на архивную задачу можно подписаться`() = runTest(dispatcher) {
        val archived = detail("""{"id":"t2","archived_at":"2026-09-01T10:00:00Z","subscribed":false}""")
        coEvery { taskRepo.detail("t2") } returns archived
        val m = TaskDetailViewModel(taskRepo, boardRepo, docRepo).also { it.load("t2", "ws1", "p1") }
        advanceUntilIdle()
        assertThat(m.state.value.readOnly).isTrue()

        m.toggleSubscription()
        advanceUntilIdle()

        coVerify(exactly = 1) { taskRepo.setSubscription("t2", true) }
    }
}
