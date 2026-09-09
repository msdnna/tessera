package website.msdnna.tessera.data.api

import com.google.common.truth.Truth.assertThat
import com.google.gson.JsonParser
import kotlinx.coroutines.runBlocking
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Before
import org.junit.Test
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import website.msdnna.tessera.data.model.CreateConferenceRequest
import website.msdnna.tessera.data.model.InviteConferenceRequest
import website.msdnna.tessera.data.model.UpdateConferenceRequest

/**
 * What the conference calls put on the wire (#2896 §1).
 *
 * A Retrofit interface fails quietly: a wrong `@Path`, a query emitted when it
 * should be omitted, or a field Gson drops all compile and all read fine in
 * review. This drives the real interface against a local server and reads the
 * request back.
 */
class ConferenceApiWireTest {
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

    private fun reply(body: String, code: Int = 200) =
        server.enqueue(MockResponse().setResponseCode(code).setBody(body))

    @Test
    fun `the status filter is omitted when absent and sent when given`() = runBlocking {
        reply("[]")
        api.conferences("ws-1")
        assertThat(server.takeRequest().path).isEqualTo("/api/workspaces/ws-1/conferences")

        reply("[]")
        api.conferences("ws-1", "live")
        assertThat(server.takeRequest().path).isEqualTo("/api/workspaces/ws-1/conferences?status=live")
    }

    @Test
    fun `an absent ttl leaves the server default alone`() = runBlocking {
        // Absent means 30 days server-side; sending 0 would mean "keep forever".
        // Gson omitting the null is the whole mechanism, so it is asserted rather
        // than assumed.
        reply("""{"id":"c-1","title":"Планёрка"}""")
        api.createConference("ws-1", CreateConferenceRequest(title = "Планёрка"))

        val req = server.takeRequest()
        assertThat(req.path).isEqualTo("/api/workspaces/ws-1/conferences")
        val body = JsonParser.parseString(req.body.readUtf8()).asJsonObject
        assertThat(body.get("title").asString).isEqualTo("Планёрка")
        assertThat(body.has("recording_ttl_days")).isFalse()
        assertThat(body.has("scheduled_at")).isFalse()

        reply("""{"id":"c-2"}""")
        api.createConference("ws-1", CreateConferenceRequest(title = "Ретро", recordingTtlDays = 0))
        val kept = JsonParser.parseString(server.takeRequest().body.readUtf8()).asJsonObject
        assertThat(kept.get("recording_ttl_days").asInt).isEqualTo(0)
    }

    @Test
    fun `a patch carries the title even when only the plan moved`() = runBlocking {
        // Unlike the document PATCH this endpoint binds `title` as required: a
        // partial edit that dropped it would come back 400, and the "save" button
        // would look broken for reasons no screen could explain.
        reply("""{"id":"c-1","title":"Планёрка"}""")
        api.updateConference(
            "c-1",
            UpdateConferenceRequest(title = "Планёрка", scheduledAt = "2026-09-07T10:00:00+03:00"),
        )

        val req = server.takeRequest()
        assertThat(req.method).isEqualTo("PATCH")
        assertThat(req.path).isEqualTo("/api/conferences/c-1")
        val body = JsonParser.parseString(req.body.readUtf8()).asJsonObject
        assertThat(body.get("title").asString).isEqualTo("Планёрка")
        assertThat(body.get("scheduled_at").asString).isEqualTo("2026-09-07T10:00:00+03:00")
    }

    @Test
    fun `attendance and moderation hang off the conference id`() = runBlocking {
        reply("""{"conference":{"id":"c-1","status":"live"},"participant":{"user_id":"u-1"}}""")
        val joined = api.joinConference("c-1")
        assertThat(server.takeRequest().path).isEqualTo("/api/conferences/c-1/join")
        assertThat(joined.conference.isLive).isTrue()

        reply("""{"conference":{"id":"c-1","status":"scheduled"},"participant":{"user_id":"u-1"}}""")
        api.leaveConference("c-1")
        assertThat(server.takeRequest().path).isEqualTo("/api/conferences/c-1/leave")

        reply("""{"id":"c-1","status":"scheduled"}""")
        api.endConference("c-1")
        assertThat(server.takeRequest().path).isEqualTo("/api/conferences/c-1/end")

        reply("""{"url":"wss://sfu","token":"ey","room":"conf-c-1","expires_in":3600}""")
        api.conferenceToken("c-1")
        val token = server.takeRequest()
        assertThat(token.method).isEqualTo("POST")
        assertThat(token.path).isEqualTo("/api/conferences/c-1/token")
    }

    @Test
    fun `an invite posts the ids as a list under the role`() = runBlocking {
        reply("[]")
        api.inviteConference("c-1", InviteConferenceRequest(listOf("u-1", "u-2")))

        val body = JsonParser.parseString(server.takeRequest().body.readUtf8()).asJsonObject
        assertThat(body.getAsJsonArray("user_ids").map { it.asString })
            .containsExactly("u-1", "u-2").inOrder()
        assertThat(body.get("role").asString).isEqualTo("member")
    }

    @Test
    fun `the chat cursor travels as a pair or not at all`() = runBlocking {
        reply("""{"messages":[],"has_more":false}""")
        api.conferenceMessages("c-1")
        assertThat(server.takeRequest().path).isEqualTo("/api/conferences/c-1/messages")

        reply("""{"messages":[],"has_more":false}""")
        api.conferenceMessages("c-1", "2026-09-05T10:05:00%2B03:00", "m-1", 50)
        val paged = server.takeRequest().path.orEmpty()
        // Both halves: the server compares a lone timestamp against a NULL uuid
        // and returns nothing at all, which reads as "no older messages".
        assertThat(paged).contains("before_at=")
        assertThat(paged).contains("before_id=m-1")
        assertThat(paged).contains("limit=50")
    }

    @Test
    fun `a message with files goes out as multipart under the fields the server reads`() = runBlocking {
        reply("""{"id":"m-1","body":"Смета","attachments":[]}""", code = 201)
        val part = okhttp3.MultipartBody.Part.createFormData(
            "files",
            "смета.pdf",
            byteArrayOf(1, 2, 3).toRequestBody("application/pdf".toMediaType()),
        )
        api.postConferenceMessageWithFiles(
            "c-1",
            "Смета".toRequestBody("text/plain".toMediaType()),
            listOf(part),
        )

        val req = server.takeRequest()
        assertThat(req.path).isEqualTo("/api/conferences/c-1/messages")
        assertThat(req.getHeader("Content-Type")).startsWith("multipart/form-data")
        val sent = req.body.readUtf8()
        // The caption rides along with the upload: posted separately it would be
        // lost whenever the file half failed.
        assertThat(sent).contains("name=\"body\"")
        assertThat(sent).contains("name=\"files\"")
    }

    @Test
    fun `deletes and downloads use their own top-level prefixes`() = runBlocking {
        // gin's tree cannot read a second :id in one path as the first in another,
        // so these deliberately do not hang off /conferences/:id.
        reply("", code = 204)
        api.deleteConferenceMessage("m-1")
        assertThat(server.takeRequest().path).isEqualTo("/api/conference-messages/m-1")

        reply("bytes")
        api.downloadConferenceAttachment("a-1").close()
        assertThat(server.takeRequest().path).isEqualTo("/api/conference-attachments/a-1")

        reply("", code = 204)
        api.deleteConferenceRecording("r-1")
        assertThat(server.takeRequest().path).isEqualTo("/api/conference-recordings/r-1")

        reply("mp4")
        api.downloadConferenceRecording("r-1").close()
        assertThat(server.takeRequest().path).isEqualTo("/api/conference-recordings/r-1/download")
    }

    @Test
    fun `recording start and stop are posts on the conference`() = runBlocking {
        reply("""{"id":"r-1","status":"active"}""", code = 201)
        val rec = api.startConferenceRecording("c-1")
        val start = server.takeRequest()
        assertThat(start.method).isEqualTo("POST")
        assertThat(start.path).isEqualTo("/api/conferences/c-1/recording/start")
        assertThat(rec.isRunning).isTrue()

        reply("""{"status":"stopping"}""", code = 202)
        api.stopConferenceRecording("c-1")
        assertThat(server.takeRequest().path).isEqualTo("/api/conferences/c-1/recording/stop")
    }
}
