package website.msdnna.tessera.ui.viewmodels

import com.google.gson.Gson
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
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import website.msdnna.tessera.data.model.SearchResults
import website.msdnna.tessera.data.repository.SearchRepository

/**
 * Что остаётся на экране, когда поиск упал (#2932).
 *
 * Провал умеют показать ровно два места — [searchPane] и строка-сноска в
 * `SearchOverlay` — и оба спрашивают `results == null`. Пока модель хранила
 * результаты прошлого запроса, это условие было истинным только для самого
 * первого поиска в сессии: дописал букву к удачному запросу, получил 500 —
 * и на экране остались прежние карточки как ответ на новый текст, без
 * сообщения и без спиннера. Веб тем же коммитом чистит выдачу в `catch`.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class SearchFailureTest {
    private val dispatcher = StandardTestDispatcher()
    private val repo = mockk<SearchRepository>()
    private val ws = "ws-1"

    @Before
    fun setUp() = Dispatchers.setMain(dispatcher)

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun hit(title: String): SearchResults = Gson().fromJson(
        """{"tasks":[{"id":"t1","number":7,"title":"$title","board_id":"b1"}],"notes":[],"documents":[]}""",
        SearchResults::class.java,
    )

    @Test
    fun `a failed search does not leave the previous query's hits on screen`() = runTest(dispatcher) {
        coEvery { repo.search(ws, "отчёт") } returns hit("Отчёт за квартал")
        coEvery { repo.search(ws, "отчёты") } throws IllegalStateException("boom")
        val vm = SearchViewModel(repo)
        vm.bind(ws)

        vm.onQueryChange("отчёт")
        advanceUntilIdle()
        assertEquals(1, vm.state.value.results?.tasks?.size)

        vm.onQueryChange("отчёты")
        advanceUntilIdle()

        val state = vm.state.value
        assertNull("выдача прошлого запроса должна уйти вместе с провалом", state.results)
        assertNotNull(state.error)
        assertEquals(SearchPane.FAILED, searchPane(state, hasHelpHits = false))
    }

    @Test
    fun `a successful search still shows its hits`() = runTest(dispatcher) {
        // Негативный контроль: починка, которая чистит выдачу всегда, оставила бы
        // пустую панель и на удачном поиске.
        coEvery { repo.search(ws, "отчёт") } returns hit("Отчёт за квартал")
        val vm = SearchViewModel(repo)
        vm.bind(ws)

        vm.onQueryChange("отчёт")
        advanceUntilIdle()

        val state = vm.state.value
        assertEquals("Отчёт за квартал", state.results?.tasks?.firstOrNull()?.title)
        assertNull(state.error)
        assertEquals(SearchPane.RESULTS, searchPane(state, hasHelpHits = false))
    }

    @Test
    fun `a superseded failure does not wipe the answer that overtook it`() = runTest(dispatcher) {
        // Провал запроса, который уже никому не нужен, приходит последним — и
        // теперь он способен обнулить выдачу. Спасает тот же счётчик seq, что и
        // от пришедшего не по порядку успеха; без него фикс менял одну тихую
        // ошибку на другую.
        coEvery { repo.search(ws, "отч") } coAnswers {
            delay(1_000)
            error("boom")
        }
        coEvery { repo.search(ws, "отчёт") } returns hit("Отчёт за квартал")
        val vm = SearchViewModel(repo)
        vm.bind(ws)

        vm.onQueryChange("отч")
        advanceTimeBy(300) // дебаунс прошёл, запрос в полёте
        vm.onQueryChange("отчёт")
        advanceUntilIdle()

        val state = vm.state.value
        assertEquals("Отчёт за квартал", state.results?.tasks?.firstOrNull()?.title)
        assertNull(state.error)
    }
}
