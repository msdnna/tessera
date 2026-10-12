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
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import retrofit2.HttpException
import retrofit2.Response
import website.msdnna.tessera.data.model.Conference
import website.msdnna.tessera.data.model.ConferenceStatus
import website.msdnna.tessera.data.repository.ConferenceRepository

/**
 * Список конференций отвечает на тот вопрос, который задали последним (#2932).
 *
 * Две двери в один и тот же промах. Вкладки фильтра («Все» → «Идут сейчас» →
 * «Запланированные») зовут `load` подряд, а `viewModel()` в ConferencesScreen
 * привязан к хосту, не к пространству, так что и смена пространства приходит
 * сюда же — старый запрос при этом живёт в `viewModelScope`, который ни тап по
 * вкладке, ни перезапуск композиции не гасят. Без проверки актуальности медленный
 * ранний ответ ложится поверх позднего: строки перестают совпадать с подсвеченной
 * вкладкой, и перечитать их некому.
 *
 * Тот же класс, что уже починен в SearchViewModel и DocumentsViewModel.
 */
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
class ConferencesLoadTest {
    private val dispatcher = StandardTestDispatcher()
    private val repo = mockk<ConferenceRepository>()

    private val live = Conference(id = "c-live", workspaceId = "ws-1", title = "Идёт", status = ConferenceStatus.LIVE)
    private val planned =
        Conference(id = "c-plan", workspaceId = "ws-1", title = "План", status = ConferenceStatus.SCHEDULED)
    private val other = Conference(id = "c-other", workspaceId = "ws-2", title = "Чужая", status = ConferenceStatus.LIVE)

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

    private fun vm() = ConferencesViewModel(repo)

    @Test
    fun `медленный ответ прошлой вкладки не ложится поверх выбранной`() = runTest(dispatcher) {
        coEvery { repo.list("ws-1", null) } coAnswers {
            delay(50)
            listOf(live, planned)
        }
        coEvery { repo.list("ws-1", ConferenceStatus.SCHEDULED) } returns listOf(planned)
        val model = vm()

        model.load("ws-1")
        advanceTimeBy(10)
        model.setFilter(ConferenceStatus.SCHEDULED)
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.filter).isEqualTo(ConferenceStatus.SCHEDULED)
        assertThat(s.list.map { it.id }).containsExactly("c-plan")
        assertThat(s.loading).isFalse()
    }

    @Test
    fun `упавшая вкладка не выдаёт строки прошлой за свои`() = runTest(dispatcher) {
        coEvery { repo.list("ws-1", null) } returns listOf(live, planned)
        coEvery { repo.list("ws-1", ConferenceStatus.ENDED) } throws forbidden()
        val model = vm()

        model.load("ws-1")
        advanceUntilIdle()
        assertThat(model.state.value.list).hasSize(2)

        model.setFilter(ConferenceStatus.ENDED)
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.list).isEmpty()
        assertThat(s.error).isNotNull()
        assertThat(s.loading).isFalse()
    }

    @Test
    fun `смена пространства уносит список и открытое лобби`() = runTest(dispatcher) {
        coEvery { repo.list("ws-1", null) } returns listOf(live)
        coEvery { repo.list("ws-2", null) } returns listOf(other)
        val model = vm()

        model.load("ws-1")
        advanceUntilIdle()
        model.open(live)
        assertThat(model.state.value.openId).isEqualTo("c-live")

        model.load("ws-2")
        advanceUntilIdle()

        val s = model.state.value
        assertThat(s.openId).isNull()
        assertThat(s.list.map { it.id }).containsExactly("c-other")
    }

    @Test
    fun `вкладка переживает смену пространства`() = runTest(dispatcher) {
        // Контроль к сбросу выше: фильтр — выбор читателя, а не свойство
        // пространства, и по нему же рисуется подсветка вкладок. Сбросив его,
        // мы бы увели подсветку из-под руки.
        coEvery { repo.list("ws-1", ConferenceStatus.LIVE) } returns listOf(live)
        coEvery { repo.list("ws-2", ConferenceStatus.LIVE) } returns listOf(other)
        coEvery { repo.list("ws-1", null) } returns listOf(live, planned)
        val model = vm()

        model.load("ws-1")
        advanceUntilIdle()
        model.setFilter(ConferenceStatus.LIVE)
        advanceUntilIdle()

        model.load("ws-2")
        advanceUntilIdle()

        assertThat(model.state.value.filter).isEqualTo(ConferenceStatus.LIVE)
        assertThat(model.state.value.list.map { it.id }).containsExactly("c-other")
    }

    @Test
    fun `поздний список прошлого пространства не дописывается в состояние`() = runTest(dispatcher) {
        coEvery { repo.list("ws-1", null) } coAnswers {
            delay(50)
            listOf(live)
        }
        coEvery { repo.list("ws-2", null) } returns listOf(other)
        val model = vm()

        model.load("ws-1")
        advanceTimeBy(10)
        model.load("ws-2")
        advanceUntilIdle()

        assertThat(model.state.value.list.map { it.id }).containsExactly("c-other")
    }
}
