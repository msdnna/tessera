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
import website.msdnna.tessera.data.model.EstimationConfig

/**
 * Переключатель «Наследовать» в диалоге оценки доходил до сервера (#2932).
 *
 * Веб сбрасывает переопределение пустым телом: `PUT …/estimation` без body —
 * `normalizeEstimationConfig` (handlers/estimation.go) отвечает на это `nil`,
 * т.е. «наследовать». Телефон собирал тот же сброс как `null` в `@Body`, но
 * Retrofit отвергает null-тело **до** выхода запроса с устройства
 * (`ParameterHandler.Body`: "Body parameter value must not be null."), так что
 * до сети дело не доходило вовсе: тумблер щёлкал, диалог закрывался, а
 * переопределение оставалось на месте до перезахода на экран.
 *
 * Лазейка у телефона есть, и она в том же хендлере: тело **без единицы** бэкенд
 * чистит ровно так же, как пустое (ветка `case ""` → `nil`). Её и шлём —
 * [EstimationConfig.INHERIT].
 */
@RunWith(RobolectricTestRunner::class)
class EstimationInheritTest {
    private lateinit var server: MockWebServer
    private lateinit var workspaces: WorkspaceRepository
    private lateinit var board: BoardRepository

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        AppContainer.init(ApplicationProvider.getApplicationContext<Context>())
        AppContainer.serverUrl = server.url("/").toString().trimEnd('/')
        RetrofitClient.reset()
        RetrofitClient.authToken = "access-1"
        workspaces = WorkspaceRepository()
        board = BoardRepository()
    }

    @After
    fun tearDown() {
        server.shutdown()
        RetrofitClient.authToken = ""
        RetrofitClient.reset()
    }

    /** Безаргументный `takeRequest()` ждёт вечно — регрессия вешала бы задачу тестов. */
    private fun awaitRequest(): RecordedRequest =
        requireNotNull(server.takeRequest(REQUEST_TIMEOUT_SECONDS, TimeUnit.SECONDS)) {
            "ожидался PUT …/estimation, но за $REQUEST_TIMEOUT_SECONDS с запроса не было"
        }

    private fun enqueue(body: String) = server.enqueue(
        MockResponse().setResponseCode(200)
            .setHeader("Content-Type", "application/json")
            .setBody(body),
    )

    @Test
    fun `clearing a project override reaches the server as a unit-less body`() = runBlocking {
        enqueue("""{"id":"p-1","name":"Проект","estimation":null}""")

        val project = workspaces.setProjectEstimation("p-1", null)

        val req = awaitRequest()
        assertThat(req.method).isEqualTo("PUT")
        assertThat(req.path).isEqualTo("/api/projects/p-1/estimation")
        assertThat(req.body.readUtf8()).isEqualTo("""{"unit":""}""")
        assertThat(project.estimation).isNull()
    }

    @Test
    fun `clearing a workspace default reaches the server as a unit-less body`() = runBlocking {
        enqueue("""{"id":"w-1","name":"Рабочее пространство","estimation":null}""")

        val ws = workspaces.setWorkspaceEstimation("w-1", null)

        val req = awaitRequest()
        assertThat(req.path).isEqualTo("/api/workspaces/w-1/estimation")
        assertThat(req.body.readUtf8()).isEqualTo("""{"unit":""}""")
        assertThat(ws.estimation).isNull()
    }

    /** Вторая дверь к тем же ручкам — экран доски. */
    @Test
    fun `the board repository clears an override the same way`() = runBlocking {
        enqueue("""{"id":"p-1","name":"Проект","estimation":null}""")

        board.setProjectEstimation("p-1", null)

        assertThat(awaitRequest().body.readUtf8()).isEqualTo("""{"unit":""}""")
    }

    /** Характеризует нетронутое: заданная конфигурация уезжает как есть. */
    @Test
    fun `a real config is sent verbatim`() = runBlocking {
        enqueue("""{"id":"p-1","name":"Проект","estimation":{"unit":"points","points_scale":"linear"}}""")

        val project = workspaces.setProjectEstimation(
            "p-1",
            EstimationConfig(unit = "points", pointsScale = "linear"),
        )

        assertThat(awaitRequest().body.readUtf8())
            .isEqualTo("""{"unit":"points","points_scale":"linear"}""")
        assertThat(project.estimation?.unit).isEqualTo("points")
        assertThat(project.estimation?.pointsScale).isEqualTo("linear")
    }

    private companion object {
        const val REQUEST_TIMEOUT_SECONDS = 5L
    }
}
