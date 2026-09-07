package website.msdnna.tessera.data.api

import com.google.common.truth.Truth.assertThat
import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Before
import org.junit.Test
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory

/**
 * Contract test for the sync-journal endpoints against **real** server output (#2918).
 *
 * `GET …/sync-runs/{runId}/actions` has shipped a keyset envelope
 * `{items, has_more, next_after_seq}` since #2616, but Android kept declaring it
 * as a bare array — Gson then threw "Expected BEGIN_ARRAY but was BEGIN_OBJECT"
 * and the journal showed a red banner with no actions at all. The same commit
 * dropped `detail` from the list rows (only `has_detail` survives), so the diff
 * dialog needs the per-row `…/detail` call.
 *
 * The payloads below are verbatim shapes of `handlers/gitlab_journal.go`
 * (`ListGitlabSyncActionsPage` row + the `{"detail": …}` reply).
 */
class GitlabJournalWireTest {
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

    private val firstPage = """
        {
          "items": [
            {
              "id": "8c2e4a20-1f3d-4a8e-9a1b-2f7c0d5e6a11",
              "seq": 0,
              "direction": "pull",
              "entity_type": "task",
              "op": "update",
              "task_id": "0f6e2c31-9b44-4b0a-8f1e-6c2d7a3b5e90",
              "gl_iid": 42,
              "summary": "#128 Починить журнал",
              "status": "ok",
              "error": "",
              "created_at": "2026-09-06T23:12:04.113221+03:00",
              "has_detail": true
            },
            {
              "id": "b41d7f6e-0c58-4d2a-9e33-7a1b8c9d0e22",
              "seq": 1,
              "direction": "push",
              "entity_type": "task",
              "op": "create",
              "task_id": null,
              "gl_iid": null,
              "summary": "#129 Новая задача",
              "status": "fail",
              "error": "502 Bad Gateway",
              "created_at": "2026-09-06T23:12:04.221007+03:00",
              "has_detail": false
            }
          ],
          "has_more": true,
          "next_after_seq": 1
        }
    """.trimIndent()

    private val lastPage = """
        {"items": [], "has_more": false, "next_after_seq": null}
    """.trimIndent()

    @Test
    fun `the actions page parses out of the keyset envelope`() = runTest {
        server.enqueue(MockResponse().setBody(firstPage))

        val page = api.gitlabSyncActions("ws-1", "run-1")

        assertThat(page.hasMore).isTrue()
        assertThat(page.nextAfterSeq).isEqualTo(1)
        val items = page.items.orEmpty()
        assertThat(items).hasSize(2)
        assertThat(items[0].summary).isEqualTo("#128 Починить журнал")
        assertThat(items[0].glIid).isEqualTo(42)
        // The list omits the diff itself and only flags that one exists.
        assertThat(items[0].hasDetail).isTrue()
        assertThat(items[0].detail).isNull()
        assertThat(items[1].status).isEqualTo("fail")
        assertThat(items[1].error).isEqualTo("502 Bad Gateway")
        assertThat(items[1].hasDetail).isFalse()
        assertThat(items[1].glIid).isNull()

        val path = server.takeRequest().path
        assertThat(path).isEqualTo("/api/workspaces/ws-1/gitlab/sync-runs/run-1/actions")
    }

    @Test
    fun `the cursor travels as after_seq`() = runTest {
        server.enqueue(MockResponse().setBody(lastPage))

        val page = api.gitlabSyncActions("ws-1", "run-1", afterSeq = 1)

        assertThat(page.items).isEmpty()
        assertThat(page.hasMore).isFalse()
        assertThat(page.nextAfterSeq).isNull()
        assertThat(server.takeRequest().path).endsWith("/actions?after_seq=1")
    }

    @Test
    fun `a row's diff is fetched on demand`() = runTest {
        server.enqueue(
            MockResponse().setBody(
                """
                {"detail": {"fields": {"title": {"before": "Старое", "after": "Новое"}}}}
                """.trimIndent(),
            ),
        )

        val detail = api.gitlabSyncActionDetail("ws-1", "run-1", "act-1").detail

        val title = detail!!.getAsJsonObject("fields").getAsJsonObject("title")
        assertThat(title.get("before").asString).isEqualTo("Старое")
        assertThat(title.get("after").asString).isEqualTo("Новое")
        assertThat(server.takeRequest().path)
            .isEqualTo("/api/workspaces/ws-1/gitlab/sync-runs/run-1/actions/act-1/detail")
    }

    /**
     * The regression itself, pinned: reading this payload as a bare array — the
     * shape Android declared until #2918 — is exactly the Gson failure users saw.
     * Keeps a future "simplification" back to `List<GitlabSyncAction>` honest.
     */
    @Test
    fun `reading the page as a bare array is the reported crash`() {
        val thrown = runCatching {
            com.google.gson.Gson().fromJson(firstPage, Array<website.msdnna.tessera.data.model.GitlabSyncAction>::class.java)
        }.exceptionOrNull()

        assertThat(thrown).isInstanceOf(com.google.gson.JsonSyntaxException::class.java)
        assertThat(thrown).hasMessageThat().contains("Expected BEGIN_ARRAY but was BEGIN_OBJECT")
    }

    /** An action with no recorded diff: the backend coalesces it to `{}`, not null. */
    @Test
    fun `an empty diff comes back as an empty object`() = runTest {
        server.enqueue(MockResponse().setBody("""{"detail": {}}"""))

        val detail = api.gitlabSyncActionDetail("ws-1", "run-1", "act-2").detail

        assertThat(detail).isNotNull()
        assertThat(detail!!.entrySet()).isEmpty()
    }
}
