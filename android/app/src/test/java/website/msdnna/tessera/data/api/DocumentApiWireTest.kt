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
import website.msdnna.tessera.data.model.CreateDocumentApprovalRequest
import website.msdnna.tessera.data.model.CreateDocumentCommentRequest
import website.msdnna.tessera.data.model.CreateDocumentRequest
import website.msdnna.tessera.data.model.UpdateDocumentContentRequest
import website.msdnna.tessera.data.model.UpdateDocumentRequest

/**
 * What the document calls actually put on the wire (#2894).
 *
 * [website.msdnna.tessera.data.model.DocumentJsonTest] checks the *response*
 * mapping; nothing checked the request until here, and a Retrofit interface
 * fails in a particularly quiet way: a wrong `@Path`, a query that is emitted
 * when it should be omitted, or a request field Gson drops all compile and all
 * look right in review. This drives the real interface against a local server
 * and reads back the request line and body.
 */
class DocumentApiWireTest {
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
    fun `the project filter is omitted when absent and sent when given`() = runBlocking {
        reply("[]")
        api.documents("ws-1")
        assertThat(server.takeRequest().path).isEqualTo("/api/workspaces/ws-1/documents")

        reply("[]")
        api.documents("ws-1", "prj-9")
        assertThat(server.takeRequest().path).isEqualTo("/api/workspaces/ws-1/documents?project_id=prj-9")
    }

    @Test
    fun `recursive delete is a flag, not a default`() = runBlocking {
        // The plain delete must not carry `recursive` at all: the server refuses a
        // document with children, and that refusal is the confirmation prompt's
        // whole reason to exist. A query that always went out would delete a
        // subtree the moment someone tapped Delete.
        // Called the way callers call it — through the declared default, so a
        // default flipped to `true` fails here rather than in production.
        reply("", code = 204)
        api.deleteDocument("doc-1")
        assertThat(server.takeRequest().path).isEqualTo("/api/documents/doc-1")

        reply("", code = 204)
        api.deleteDocument("doc-1", true)
        assertThat(server.takeRequest().path).isEqualTo("/api/documents/doc-1?recursive=true")
    }

    @Test
    fun `a content save carries the seen timestamp and the connection id`() = runBlocking {
        reply("""{"id":"doc-1","updated_at":"2026-09-05T00:28:25.716274+03:00","preview":"Регламент"}""")
        val content = JsonParser.parseString("""{"type":"doc","content":[]}""")
        val saved = api.updateDocumentContent(
            "doc-1",
            UpdateDocumentContentRequest(content, "2026-09-05T00:28:25.681779+03:00", "conn-7"),
        )

        val req = server.takeRequest()
        assertThat(req.method).isEqualTo("PATCH")
        assertThat(req.path).isEqualTo("/api/documents/doc-1/content")
        val body = JsonParser.parseString(req.body.readUtf8()).asJsonObject
        // Without updated_at the server cannot detect a lost race and would
        // silently overwrite a colleague; without conn_id our own save bounces
        // back at us through the room socket (#2729).
        assertThat(body.get("updated_at").asString).isEqualTo("2026-09-05T00:28:25.681779+03:00")
        assertThat(body.get("conn_id").asString).isEqualTo("conn-7")
        assertThat(body.getAsJsonObject("content").get("type").asString).isEqualTo("doc")
        assertThat(saved.updatedAt).isEqualTo("2026-09-05T00:28:25.716274+03:00")
    }

    @Test
    fun `an absent field is left out of a metadata patch`() = runBlocking {
        reply("""{"id":"doc-1","title":"Новое имя"}""")
        api.updateDocument("doc-1", UpdateDocumentRequest(title = "Новое имя"))

        val body = JsonParser.parseString(server.takeRequest().body.readUtf8()).asJsonObject
        assertThat(body.keySet()).containsExactly("title")
        // Gson omitting nulls is what makes a partial PATCH partial: sending
        // "icon": null here would clear the icon of every renamed document.
        assertThat(body.has("icon")).isFalse()
        assertThat(body.has("position")).isFalse()
    }

    @Test
    fun `creating from a template posts the template id`() = runBlocking {
        reply("""{"id":"doc-2","title":"Протокол"}""")
        api.createDocument("ws-1", CreateDocumentRequest(title = "Протокол", templateId = "tpl-3"))

        val req = server.takeRequest()
        assertThat(req.path).isEqualTo("/api/workspaces/ws-1/documents")
        val body = JsonParser.parseString(req.body.readUtf8()).asJsonObject
        assertThat(body.get("template_id").asString).isEqualTo("tpl-3")
    }

    @Test
    fun `a reply names its parent and an approval sends its route`() = runBlocking {
        reply("""{"id":"c-2","body":"Согласен"}""")
        api.createDocumentComment("doc-1", CreateDocumentCommentRequest(body = "Согласен", parentId = "c-1"))
        val comment = JsonParser.parseString(server.takeRequest().body.readUtf8()).asJsonObject
        assertThat(comment.get("parent_id").asString).isEqualTo("c-1")

        reply("""{"id":"a-1","status":"pending"}""")
        api.createDocumentApproval(
            "doc-1",
            CreateDocumentApprovalRequest(approvers = listOf("u-1", "u-2"), title = "Утвердить"),
        )
        val approval = JsonParser.parseString(server.takeRequest().body.readUtf8()).asJsonObject
        assertThat(approval.getAsJsonArray("approvers").map { it.asString })
            .containsExactly("u-1", "u-2").inOrder()
        assertThat(approval.get("mode").asString).isEqualTo("sequential")
    }

    @Test
    fun `the flat routes hang off their own prefixes, not the document`() = runBlocking {
        // Comments, versions, templates and links are edited through top-level
        // `/document-*` paths — an id in the wrong segment would 404 at runtime.
        reply("""{"id":"c-1"}""")
        api.resolveDocumentComment("c-1", website.msdnna.tessera.data.model.ResolveDocumentCommentRequest(false))
        assertThat(server.takeRequest().path).isEqualTo("/api/document-comments/c-1/resolve")

        reply("""{"id":"doc-1"}""")
        api.restoreDocumentVersion("v-1")
        assertThat(server.takeRequest().path).isEqualTo("/api/document-versions/v-1/restore")

        reply("""{"id":"s-1"}""")
        api.decideDocumentApproval(
            "a-1",
            website.msdnna.tessera.data.model.DecideDocumentApprovalRequest("approved"),
        )
        assertThat(server.takeRequest().path).isEqualTo("/api/document-approvals/a-1/decide")

        reply("", code = 204)
        api.deleteDocumentTaskLink("l-1")
        assertThat(server.takeRequest().path).isEqualTo("/api/document-task-links/l-1")
    }

    @Test
    fun `an export asks for its format and comes back as raw bytes`() = runBlocking {
        reply("%PDF-1.7 fake")
        val bytes = api.exportDocument("doc-1", "pdf").bytes()
        assertThat(server.takeRequest().path).isEqualTo("/api/documents/doc-1/export?format=pdf")
        // Streaming rather than a parsed body: a PDF read through Gson would come
        // back corrupted in a way only the downloaded file shows.
        assertThat(String(bytes)).startsWith("%PDF")
    }

    @Test
    fun `an upload goes out as multipart under the field the server reads`() = runBlocking {
        reply("""{"url":"/api/documents/asset?token=x"}""")
        val part = okhttp3.MultipartBody.Part.createFormData(
            "file",
            "снимок.png",
            byteArrayOf(1, 2, 3).toRequestBody("image/png".toMediaType()),
        )
        api.uploadDocumentAsset("doc-1", part)

        val req = server.takeRequest()
        assertThat(req.path).isEqualTo("/api/documents/doc-1/assets")
        assertThat(req.getHeader("Content-Type")).startsWith("multipart/form-data")
        assertThat(req.body.readUtf8()).contains("name=\"file\"")
    }
}
