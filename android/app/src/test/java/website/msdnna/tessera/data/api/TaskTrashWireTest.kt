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

/**
 * The task trash on the wire (#3004). The undo of a delete is a POST to
 * `/tasks/:id/restore` — the archive's restore on the same path is a PATCH, and
 * swapping the two would «restore» a deleted task out of an archive it was
 * never in. The archive's «Удалить навсегда» purges after the delete.
 */
class TaskTrashWireTest {
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

    @Test
    fun `the undo of a delete is a POST to restore`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id":"t-1"}"""))
        api.untrashTask("t-1")

        val req = server.takeRequest()
        assertThat(req.method).isEqualTo("POST")
        assertThat(req.requestUrl?.encodedPath).isEqualTo("/api/tasks/t-1/restore")
    }

    @Test
    fun `the archive restore stays a PATCH`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(204))
        api.restoreTask("t-1")

        assertThat(server.takeRequest().method).isEqualTo("PATCH")
    }

    @Test
    fun `purge deletes the trashed task for good`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(204))
        api.purgeTask("t-1")

        val req = server.takeRequest()
        assertThat(req.method).isEqualTo("DELETE")
        assertThat(req.requestUrl?.encodedPath).isEqualTo("/api/tasks/t-1/purge")
    }
}
