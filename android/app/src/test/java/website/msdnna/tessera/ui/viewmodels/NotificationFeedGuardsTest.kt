package website.msdnna.tessera.ui.viewmodels

import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.CompletableDeferred
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
import website.msdnna.tessera.data.model.Notification
import website.msdnna.tessera.data.repository.NotificationRepository

/**
 * Лента уведомлений и колокольчик: три гварда, которые веб-стор
 * (`frontend/src/stores/notifications.js`) получил в #2932, а Android — нет.
 *
 * 1. **Порядок ответов ≠ порядок запросов.** Ленту читают на старте и заново по
 *    каждому событию сокета, то есть ровно тогда, когда связь мигает. Отмена
 *    `reloadJob` снимает только задержку дребезга: сам `load()` уходит в
 *    отдельную корутину, и два запроса спокойно живут в полёте вместе. Без
 *    порядкового номера на экране остаётся тот, что ответил последним.
 * 2. **Оптимистичная отметка переживает чужую перезагрузку.** Прочитанность
 *    движется только в одну сторону, поэтому снимок ленты, снятый сервером до
 *    тапа, не имеет права вернуть строку в непрочитанные.
 * 3. **Отказ сервера откатывается.** Иначе счётчик падает в ноль, сервер держит
 *    строки непрочитанными, и лента врёт до следующей загрузки.
 *
 * ViewModel в `init` ещё и лезет в DataStore за id устройства — это отдельная
 * корутина на настоящем Dispatchers.IO, виртуальное время `runTest` её не ждёт и
 * на проверяемое поведение она не влияет.
 */
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
class NotificationFeedGuardsTest {
    private val dispatcher = StandardTestDispatcher()
    private val repo = mockk<NotificationRepository>(relaxUnitFun = true)

    private val old = Notification(id = "n-1", kind = "comment", text = "старая")
    private val fresh = Notification(id = "n-2", kind = "mention", text = "свежая")

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        AppContainer.init(ApplicationProvider.getApplicationContext())
        coEvery { repo.list() } returns emptyList()
    }

    @After
    fun tearDown() = Dispatchers.resetMain()

    @Test
    fun `ответ протухшей загрузки не затирает свежую ленту`() = runTest(dispatcher) {
        val gate = CompletableDeferred<Unit>()
        var calls = 0
        coEvery { repo.list() } coAnswers {
            calls++
            if (calls == 1) {
                gate.await()
                listOf(old)
            } else {
                listOf(fresh)
            }
        }

        // Первая загрузка уходит из init и виснет на шлюзе.
        val model = NotificationViewModel(repo)
        advanceUntilIdle()

        // Вторую спросили позже — её ответ и должен остаться на экране.
        model.load()
        advanceUntilIdle()
        assertThat(model.state.value.items.map { it.id }).containsExactly("n-2")

        // А теперь отвечает первая. Спросили её раньше — значит она протухла.
        gate.complete(Unit)
        advanceUntilIdle()
        assertThat(model.state.value.items.map { it.id }).containsExactly("n-2")
    }

    @Test
    fun `перезагрузка не воскрешает прочитанную строку`() = runTest(dispatcher) {
        coEvery { repo.list() } returns listOf(old)
        val model = NotificationViewModel(repo)
        advanceUntilIdle()

        model.markRead(old)
        advanceUntilIdle()
        assertThat(model.state.value.unread).isEqualTo(0)

        // Сервер снял снимок до отметки — строка приходит непрочитанной.
        model.load()
        advanceUntilIdle()
        assertThat(model.state.value.unread).isEqualTo(0)
    }

    @Test
    fun `отказ на markRead возвращает строку в непрочитанные`() = runTest(dispatcher) {
        coEvery { repo.list() } returns listOf(old)
        coEvery { repo.markRead(any()) } throws IllegalStateException("нет сети")

        val model = NotificationViewModel(repo)
        advanceUntilIdle()

        model.markRead(old)
        advanceUntilIdle()

        assertThat(model.state.value.unread).isEqualTo(1)
        assertThat(model.state.value.error).isNotNull()
    }

    @Test
    fun `отказ на markAllRead возвращает счётчик`() = runTest(dispatcher) {
        coEvery { repo.list() } returns listOf(old, fresh)
        coEvery { repo.markAllRead() } throws IllegalStateException("нет сети")

        val model = NotificationViewModel(repo)
        advanceUntilIdle()
        assertThat(model.state.value.unread).isEqualTo(2)

        model.markAllRead()
        advanceUntilIdle()

        assertThat(model.state.value.unread).isEqualTo(2)
        assertThat(model.state.value.error).isNotNull()
    }

    @Test
    fun `уже прочитанную строку markAllRead назад не тащит`() = runTest(dispatcher) {
        coEvery { repo.list() } returns listOf(old.copy(readAt = "2026-01-01T00:00:00Z"), fresh)
        coEvery { repo.markAllRead() } throws IllegalStateException("нет сети")

        val model = NotificationViewModel(repo)
        advanceUntilIdle()

        model.markAllRead()
        advanceUntilIdle()

        // Откат трогает только то, что перекрыл оптимистичный проход.
        assertThat(model.state.value.items.first { it.id == "n-1" }.isUnread).isFalse()
        assertThat(model.state.value.items.first { it.id == "n-2" }.isUnread).isTrue()
    }
}
