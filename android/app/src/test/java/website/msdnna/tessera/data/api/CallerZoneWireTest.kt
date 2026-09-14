package website.msdnna.tessera.data.api

import com.google.common.truth.Truth.assertThat
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Before
import org.junit.Test
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import website.msdnna.tessera.data.model.CreateCommentRequest
import website.msdnna.tessera.data.model.CreateGitlabIssueRequest
import website.msdnna.tessera.data.model.PreviewCommandsRequest

/**
 * The `tz` query on every call the server answers BY DAY (#2932).
 *
 * `callerLocation` (backend handlers/api.go) reads the zone off the request
 * first, and only then off the stored profile — because an empty profile zone
 * means "the client's own", which the server cannot re-derive. Without it the
 * home tiles are bucketed in the *server's* calendar while the list under them is
 * filtered in the device's; a «/due сегодня» typed on the phone lands on the
 * server's day; and a task exported to GitLab is dated in the server's day too.
 */
class CallerZoneWireTest {
    private lateinit var server: MockWebServer
    private lateinit var api: ApiService

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        api = Retrofit.Builder()
            .baseUrl(server.url("/api/"))
            .addConverterFactory(GsonConverterFactory.create())
            .build()
            .create(ApiService::class.java)
    }

    @After
    fun tearDown() = server.shutdown()

    private fun reply(body: String) = server.enqueue(MockResponse().setResponseCode(200).setBody(body))

    @Test
    fun `the home summary carries the caller zone`() = runBlocking {
        reply("{}")
        api.workspaceSummary("ws-1", "Europe/Moscow")

        val req = server.takeRequest()
        assertThat(req.requestUrl?.encodedPath).isEqualTo("/api/workspaces/ws-1/summary")
        assertThat(req.requestUrl?.queryParameter("tz")).isEqualTo("Europe/Moscow")
    }

    @Test
    fun `a posted comment carries the caller zone`() = runBlocking {
        // «/due сегодня» in the body is resolved server-side against this zone.
        reply("""{"comment":null}""")
        api.createComment("t-1", CreateCommentRequest(body = "/due сегодня"), "Asia/Tokyo")

        val req = server.takeRequest()
        assertThat(req.requestUrl?.encodedPath).isEqualTo("/api/tasks/t-1/comments")
        assertThat(req.requestUrl?.queryParameter("tz")).isEqualTo("Asia/Tokyo")
    }

    @Test
    fun `the command dry-run carries the same zone as the real post`() = runBlocking {
        // Or the «Будет применено» hint names a different day than the command sets.
        reply("""{"commands":[]}""")
        api.previewCommands("t-1", PreviewCommandsRequest("/due сегодня"), "Asia/Tokyo")

        val req = server.takeRequest()
        assertThat(req.requestUrl?.encodedPath).isEqualTo("/api/tasks/t-1/commands/preview")
        assertThat(req.requestUrl?.queryParameter("tz")).isEqualTo("Asia/Tokyo")
    }

    @Test
    fun `creating a GitLab issue carries the caller zone`() = runBlocking {
        // The issue is born with the task's due date rendered as a bare
        // `YYYY-MM-DD` (backend taskDateStr) — a day, so it is read in the zone
        // the request names. Without one the server falls back to the server's
        // own calendar and, east of it, dates the issue a day early.
        reply("""{"link":null,"attachments":null}""")
        api.createGitlabIssue("t-1", CreateGitlabIssueRequest(), "Europe/Moscow")

        val req = server.takeRequest()
        assertThat(req.requestUrl?.encodedPath).isEqualTo("/api/tasks/t-1/gitlab-issue")
        assertThat(req.requestUrl?.queryParameter("tz")).isEqualTo("Europe/Moscow")
    }

    @Test
    fun `an unknown zone is omitted rather than sent empty`() = runBlocking {
        // An empty `tz` is not a zone: it would load nowhere and cost the server
        // its next fallback (the stored profile zone), so the caller passes null.
        reply("{}")
        api.workspaceSummary("ws-1", null)

        assertThat(server.takeRequest().requestUrl?.queryParameter("tz")).isNull()
    }
}
