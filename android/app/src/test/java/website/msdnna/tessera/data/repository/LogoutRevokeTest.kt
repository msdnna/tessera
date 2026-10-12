package website.msdnna.tessera.data.repository

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient

/**
 * «Выйти» отзывает сессию на сервере, а не только на телефоне (#2932).
 *
 * Refresh-строка живёт 30 суток (backend `auth.RefreshTokenTTL`) и продлевает
 * себя ротацией, поэтому без `POST /auth/logout` выход заканчивался на этом
 * экране: снятая с устройства копия токена переживала его на месяц. Веб отзывает
 * с первого дня (`stores/auth.js` logout), и сам бэкенд называет это состояние
 * в комментарии к ручке.
 *
 * Обратная сторона — пути, где сессию уже отверг сервер: там отзывать нечего, и
 * лишний round-trip только задержал бы экран входа. Их разводит [clearSession].
 */
@RunWith(RobolectricTestRunner::class)
class LogoutRevokeTest {
    private lateinit var server: MockWebServer
    private lateinit var repo: AuthRepository

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        AppContainer.init(ApplicationProvider.getApplicationContext<Context>())
        AppContainer.serverUrl = server.url("/").toString().trimEnd('/')
        RetrofitClient.reset()
        RetrofitClient.authToken = "access-1"
        RetrofitClient.refreshToken = "refresh-1"
        repo = AuthRepository()
    }

    /**
     * Ограничение по времени здесь принципиально: безаргументный `takeRequest()`
     * ждёт вечно, и регрессия, пропускающая отзыв, вешала бы всю задачу тестов
     * вместо того, чтобы покраснеть.
     */
    private fun awaitRequest(): RecordedRequest =
        requireNotNull(server.takeRequest(REQUEST_TIMEOUT_SECONDS, TimeUnit.SECONDS)) {
            "ожидался POST /api/auth/logout, но за $REQUEST_TIMEOUT_SECONDS с запроса не было"
        }

    @After
    fun tearDown() {
        server.shutdown()
        RetrofitClient.authToken = ""
        RetrofitClient.refreshToken = ""
        RetrofitClient.reset()
    }

    @Test
    fun `signing out revokes the refresh token on the server`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(204))

        repo.logout()

        val req = awaitRequest()
        assertThat(req.method).isEqualTo("POST")
        assertThat(req.path).isEqualTo("/api/auth/logout")
        // Ровно тот токен, что лежал в клиенте: сервер ищет строку по его хэшу.
        assertThat(req.body.readUtf8()).contains("\"refresh_token\":\"refresh-1\"")
        assertThat(RetrofitClient.authToken).isEmpty()
        assertThat(RetrofitClient.refreshToken).isEmpty()
    }

    @Test
    fun `a failed revoke still ends the session locally`() = runBlocking {
        // Best-effort: офлайн или упавший сервер не должны оставлять человека
        // залогиненным на своём же устройстве.
        server.enqueue(MockResponse().setResponseCode(500))

        repo.logout()

        assertThat(awaitRequest().path).isEqualTo("/api/auth/logout")
        assertThat(RetrofitClient.refreshToken).isEmpty()
    }

    @Test
    fun `clearing an expired session asks the server for nothing`() = runBlocking {
        repo.clearSession()

        assertThat(server.requestCount).isEqualTo(0)
        assertThat(RetrofitClient.refreshToken).isEmpty()
    }

    @Test
    fun `without a stored token there is nothing to revoke`() = runBlocking {
        RetrofitClient.refreshToken = ""

        repo.logout()

        assertThat(server.requestCount).isEqualTo(0)
    }

    private companion object {
        const val REQUEST_TIMEOUT_SECONDS = 5L
    }
}
