package website.msdnna.tessera.data.api

import com.google.common.truth.Truth.assertThat
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import okhttp3.mockwebserver.SocketPolicy
import org.junit.After
import org.junit.Before
import org.junit.Test

/**
 * Мигнувшая сеть — не конец сессии (#2932).
 *
 * Тихое обновление токена разводит две разные неудачи, и цена ошибки
 * несимметрична: перепутав их, клиент чистит токены и уводит человека на экран
 * входа с требованием ввести пароль — из-за перезапуска бэкенда или секунды без
 * сети. Бэкенд специально отвечает на такой случай 503, а не 401 (`handlers/
 * auth.go` Refresh: «A database blip is not a verdict on the session»), и веб
 * научился это читать в #2750 (`frontend/src/api/index.js` refreshFailure).
 *
 * Обратная сторона так же важна: настоящий отказ (просроченная строка — 401,
 * отключённый аккаунт — 403) обязан заканчиваться выходом, иначе клиент будет
 * вечно биться в сервер мёртвым токеном.
 */
class RefreshBlipTest {
    private lateinit var server: MockWebServer
    private var loggedOut = false

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        RetrofitClient.reset()
        RetrofitClient.authToken = "access-stale"
        RetrofitClient.refreshToken = "refresh-1"
        loggedOut = false
        RetrofitClient.onUnauthorized = { loggedOut = true }
    }

    @After
    fun tearDown() {
        server.shutdown()
        RetrofitClient.onUnauthorized = null
        RetrofitClient.onTokensRefreshed = null
        RetrofitClient.authToken = ""
        RetrofitClient.refreshToken = ""
        RetrofitClient.reset()
    }

    private fun service(): ApiService =
        RetrofitClient.getService(server.url("/").toString().trimEnd('/'))

    /** Ограничение по времени: не дождавшись запроса, безаргументный
     *  `takeRequest()` повесил бы всю задачу тестов вместо красного теста. */
    private fun awaitRequest(): RecordedRequest =
        requireNotNull(server.takeRequest(REQUEST_TIMEOUT_SECONDS, TimeUnit.SECONDS)) {
            "ожидался запрос, но за $REQUEST_TIMEOUT_SECONDS с его не было"
        }

    private fun callMe(): Result<Unit> = runBlocking {
        runCatching { service().me() }.map { }
    }

    @Test
    fun `a storage blip during refresh keeps the session`() {
        server.enqueue(MockResponse().setResponseCode(401))
        server.enqueue(
            MockResponse().setResponseCode(503).setBody("""{"error":"storage unavailable"}"""),
        )

        assertThat(callMe().isFailure).isTrue()

        assertThat(awaitRequest().path).isEqualTo("/api/auth/me")
        assertThat(awaitRequest().path).isEqualTo("/api/auth/refresh")
        // Главное: токены на месте и выхода не было — вернётся сеть, вернётся и сессия.
        assertThat(RetrofitClient.refreshToken).isEqualTo("refresh-1")
        assertThat(RetrofitClient.authToken).isEqualTo("access-stale")
        assertThat(loggedOut).isFalse()
    }

    @Test
    fun `an unreachable server keeps the session`() {
        server.enqueue(MockResponse().setResponseCode(401))
        // Ответа на обновление не будет вовсе: сервер закрывает соединение.
        server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AT_START))

        assertThat(callMe().isFailure).isTrue()

        assertThat(RetrofitClient.refreshToken).isEqualTo("refresh-1")
        assertThat(loggedOut).isFalse()
    }

    @Test
    fun `a refused refresh ends the session`() {
        server.enqueue(MockResponse().setResponseCode(401))
        server.enqueue(
            MockResponse().setResponseCode(401).setBody("""{"error":"token expired or revoked"}"""),
        )

        assertThat(callMe().isFailure).isTrue()

        assertThat(RetrofitClient.refreshToken).isEmpty()
        assertThat(RetrofitClient.authToken).isEmpty()
        assertThat(loggedOut).isTrue()
    }

    @Test
    fun `a deactivated account ends the session`() {
        // 403 — тот же ответ, что Login даёт отключённому аккаунту: вердикт, не заминка.
        server.enqueue(MockResponse().setResponseCode(401))
        server.enqueue(
            MockResponse().setResponseCode(403).setBody("""{"error":"account is deactivated"}"""),
        )

        assertThat(callMe().isFailure).isTrue()

        assertThat(RetrofitClient.refreshToken).isEmpty()
        assertThat(loggedOut).isTrue()
    }

    @Test
    fun `a successful refresh retries the request with the new token`() {
        server.enqueue(MockResponse().setResponseCode(401))
        server.enqueue(
            MockResponse().setBody(
                """{"access_token":"access-fresh","refresh_token":"refresh-2"}""",
            ),
        )
        server.enqueue(MockResponse().setBody("""{"user":{"id":"u1"}}"""))

        assertThat(callMe().isSuccess).isTrue()

        awaitRequest()
        awaitRequest()
        assertThat(awaitRequest().getHeader("Authorization")).isEqualTo("Bearer access-fresh")
        assertThat(RetrofitClient.refreshToken).isEqualTo("refresh-2")
        assertThat(loggedOut).isFalse()
    }

    @Test
    fun `only a client refusal counts as a verdict on the session`() {
        assertThat(isSessionVerdict(401)).isTrue()
        assertThat(isSessionVerdict(403)).isTrue()
        assertThat(isSessionVerdict(400)).isTrue()
        // Про момент, а не про сессию — как и на вебе.
        assertThat(isSessionVerdict(408)).isFalse()
        assertThat(isSessionVerdict(429)).isFalse()
        assertThat(isSessionVerdict(500)).isFalse()
        assertThat(isSessionVerdict(502)).isFalse()
        assertThat(isSessionVerdict(503)).isFalse()
    }

    private companion object {
        const val REQUEST_TIMEOUT_SECONDS = 5L
    }
}
