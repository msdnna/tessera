package website.msdnna.tessera.e2e

import android.os.Build
import com.google.gson.Gson
import java.util.UUID
import java.util.concurrent.TimeUnit
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.junit.Assume
import website.msdnna.tessera.data.model.AddTagRequest
import website.msdnna.tessera.data.model.AuthResponse
import website.msdnna.tessera.data.model.Board
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.Comment
import website.msdnna.tessera.data.model.CreateGroupRequest
import website.msdnna.tessera.data.model.CreateProjectRequest
import website.msdnna.tessera.data.model.CreateTagRequest
import website.msdnna.tessera.data.model.CreateTaskRequest
import website.msdnna.tessera.data.model.Document
import website.msdnna.tessera.data.model.NameRequest
import website.msdnna.tessera.data.model.Project
import website.msdnna.tessera.data.model.ProjectGroup
import website.msdnna.tessera.data.model.RegisterRequest
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.data.model.TaskDetail
import website.msdnna.tessera.data.model.UpdateTaskRequest
import website.msdnna.tessera.data.model.User
import website.msdnna.tessera.data.model.Workspace

/**
 * The live backend the e2e tier runs against — a throwaway server on :8092
 * backed by the `tessera_test` database (`make e2e-backend-up`). The production
 * `tessera` database is never touched.
 *
 * Seeding goes through this raw OkHttp client rather than the app's Retrofit
 * stack on purpose: a spec that fails must point at the app, not at the fixture
 * that set it up. Every run registers its own user and workspace, so runs never
 * collide and `tessera_test` needs no cleanup between them.
 */
object E2eBackend {
    /**
     * Whether the suite is running on a real device/emulator rather than on the
     * host JVM under Robolectric.
     *
     * The two tiers share this harness, and a few things genuinely differ between
     * them — where the backend lives ([defaultUrl]) and whether waiting in
     * wall-clock time means anything (`ComposeDrag.holdPastLongPress`).
     * Robolectric's own [Build.FINGERPRINT] tells them apart without a
     * compile-time flag or a second copy of the harness.
     *
     * **Declared first on purpose:** an object initialises its properties in
     * source order, so a [serverUrl] resolved above this line would read a
     * still-false `onDevice` and send the instrumented tier at `localhost` —
     * where nothing answers, which skips the suite and reads as green.
     */
    val onDevice: Boolean = Build.FINGERPRINT != ROBOLECTRIC_FINGERPRINT

    /** Overridable for CI (`-Dtessera.e2e.url=…`); defaults per tier, see [defaultUrl]. */
    val serverUrl: String =
        System.getProperty("tessera.e2e.url")
            ?: System.getenv("TESSERA_E2E_URL")
            ?: defaultUrl()

    /**
     * Where the throwaway backend lives, as seen from the tier that is running.
     *
     * The JVM tier is a host process, so the server is plain `localhost`. The
     * instrumented tier runs *inside* the device, where `localhost` is the device
     * itself and the host is reachable through the emulator's `10.0.2.2` alias
     * (already the project's dev-backend address, `android/ARCHITECTURE.md:52`).
     * Getting this wrong is not loud — an unreachable address makes
     * [requireBackend] skip the suite, which reads as green — so the JVM tier
     * asserts its own resolution in `HarnessE2eTest`.
     */
    private fun defaultUrl(): String =
        if (onDevice) "http://10.0.2.2:8092" else "http://localhost:8092"

    /** Robolectric's own value for [Build.FINGERPRINT]. */
    private const val ROBOLECTRIC_FINGERPRINT = "robolectric"

    val apiUrl: String = serverUrl.trimEnd('/') + "/api/"

    /** Password used for every seeded account; specs that log in through the UI need it. */
    const val PASSWORD = "e2e-passw0rd"

    private const val JSON_TIMEOUT_S = 20L

    private val json = "application/json".toMediaType()
    private val gson = Gson()

    private val http = OkHttpClient.Builder()
        .connectTimeout(5, TimeUnit.SECONDS)
        .readTimeout(JSON_TIMEOUT_S, TimeUnit.SECONDS)
        .callTimeout(JSON_TIMEOUT_S, TimeUnit.SECONDS)
        .build()

    /** Probed once per JVM: a health check that costs one request, not one per test. */
    val available: Boolean by lazy {
        runCatching {
            http.newCall(Request.Builder().url(apiUrl + "health").build())
                .execute()
                .use { it.isSuccessful }
        }.getOrDefault(false)
    }

    /**
     * Skips (not fails) the calling test when no e2e backend is listening, so a
     * plain `make test-android` on a machine without Postgres stays green.
     */
    fun requireBackend() {
        Assume.assumeTrue(
            "e2e backend is not reachable at $serverUrl — run `make e2e-backend-up`",
            available,
        )
    }

    // ── seeding ────────────────────────────────────────────────────────────

    /** A registered account plus its token pair, ready to hand to the app. */
    data class Account(
        val email: String,
        val password: String,
        val user: User,
        val accessToken: String,
        val refreshToken: String,
    )

    /** A workspace seeded down to a board with its four default columns. */
    data class Fixture(
        val account: Account,
        val workspace: Workspace,
        val group: ProjectGroup,
        val project: Project,
        val board: Board,
        val columns: List<BoardColumn>,
    ) {
        /** Leftmost column — where a freshly created task lands. */
        val firstColumn: BoardColumn get() = columns.first()
    }

    /** Registers a fresh account. The local part is unique per call, so parallel
     *  runs against the same `tessera_test` never fight over an email. */
    fun registerAccount(namePrefix: String = "E2E"): Account {
        val email = "e2e+${UUID.randomUUID()}@test.local"
        val res = post<AuthResponse>(
            path = "auth/register",
            body = RegisterRequest(email = email, name = "$namePrefix bot", password = PASSWORD),
        )
        return Account(
            email = email,
            password = PASSWORD,
            user = res.user ?: User(),
            accessToken = res.accessToken,
            refreshToken = res.refreshToken,
        )
    }

    /**
     * Seeds workspace → group → project → board for [account]. A board comes
     * back from the backend with four default columns already created
     * (`handlers/boards.go defaultColumns`), so we only read them back.
     */
    fun seedBoard(account: Account, label: String = "e2e"): Fixture {
        val token = account.accessToken
        val workspace = post<Workspace>("workspaces", NameRequest("$label ws"), token)
        val group = post<ProjectGroup>(
            "workspaces/${workspace.id}/groups",
            CreateGroupRequest(name = "$label group"),
            token,
        )
        val project = post<Project>(
            "workspaces/${workspace.id}/projects",
            CreateProjectRequest(name = "$label project", groupId = group.id),
            token,
        )
        val board = post<Board>("projects/${project.id}/boards", NameRequest("$label board"), token)
        val columns = getList<BoardColumn>("boards/${board.id}/columns", token)
        check(columns.isNotEmpty()) { "seeded board ${board.id} came back without default columns" }
        return Fixture(account, workspace, group, project, board, columns)
    }

    /** Creates a task from the outside — for asserting that the app renders (or
     *  live-updates over the websocket) data it did not create itself. */
    fun createTask(fixture: Fixture, title: String, column: BoardColumn = fixture.firstColumn): Task =
        post(
            "boards/${fixture.board.id}/tasks",
            CreateTaskRequest(columnId = column.id, title = title),
            fixture.account.accessToken,
        )

    /** Adds a column from the outside — the realtime counterpart of the app's own
     *  end-of-board tile. */
    fun createColumn(fixture: Fixture, name: String): BoardColumn =
        post("boards/${fixture.board.id}/columns", NameRequest(name), fixture.account.accessToken)

    /** Creates a project tag. Tags are what the board's killer feature groups by,
     *  and a namespaced name («app::android») is what turns into a prefix lane. */
    fun createTag(fixture: Fixture, name: String, color: String = "#7c5cff"): Tag =
        post(
            "projects/${fixture.project.id}/tags",
            CreateTagRequest(name = name, color = color),
            fixture.account.accessToken,
        )

    /** Puts a tag on a task. The endpoint answers 204, so this reads no body —
     *  going through [post] would fail on parsing the empty response. */
    fun addTaskTag(fixture: Fixture, taskId: String, tagId: String) {
        val req = Request.Builder()
            .url(apiUrl + "tasks/$taskId/tags")
            .post(gson.toJson(AddTagRequest(tagId)).toRequestBody(json))
            .header("Authorization", "Bearer ${fixture.account.accessToken}")
            .build()
        http.newCall(req).execute().use { resp ->
            check(resp.isSuccessful) {
                "e2e seed tag $tagId on task $taskId failed: HTTP ${resp.code} ${resp.body.string()}"
            }
        }
    }

    /** Renames a task from the outside. The endpoint is a full replace (title is
     *  mandatory), so the rest of the task is carried over from its current state
     *  rather than silently reset to defaults. */
    fun renameTask(fixture: Fixture, taskId: String, title: String): Task {
        val current = task(fixture, taskId)
        return patch(
            "tasks/$taskId",
            UpdateTaskRequest(
                title = title,
                description = current.description,
                priority = current.priority,
                dueDate = current.dueDate,
                startDate = current.startDate,
                estimate = current.estimate,
                completed = current.isCompleted,
                recurrence = current.recurrence,
            ),
            fixture.account.accessToken,
        )
    }

    // ── reading state back ─────────────────────────────────────────────────
    //
    // A spec that drives the UI has to confirm the write reached Postgres, not
    // just that the screen redrew: an optimistic update paints a card that a
    // failed request never persisted, and a UI-only assertion would pass. These
    // read the server's own view of the board.

    /** Top-level cards on the seeded board, as the server has them. */
    fun tasks(fixture: Fixture): List<Task> =
        getList("boards/${fixture.board.id}/tasks", fixture.account.accessToken)

    /** Columns on the seeded board, as the server has them. */
    fun columns(fixture: Fixture): List<BoardColumn> =
        getList("boards/${fixture.board.id}/columns", fixture.account.accessToken)

    /** One task's full detail — the modal's own read, so a spec checks the same
     *  fields the screen edits (description, priority, column). */
    fun task(fixture: Fixture, taskId: String): TaskDetail =
        get("tasks/$taskId", fixture.account.accessToken)

    /** Comments on a task, as the server has them. */
    fun comments(fixture: Fixture, taskId: String): List<Comment> =
        getList("tasks/$taskId/comments", fixture.account.accessToken)

    // ── documents (#2735) ──────────────────────────────────────────────────
    //
    // Seeded as raw maps rather than through request models: the Android client
    // is read-only by design, so there is nothing to reuse and adding write
    // models here would imply an app capability that does not exist.

    /** Creates a document, optionally nested under [parentId]. */
    fun createDocument(
        fixture: Fixture,
        title: String,
        parentId: String? = null,
        icon: String = "",
    ): Document {
        val body = mutableMapOf<String, Any>("title" to title, "icon" to icon)
        if (parentId != null) body["parent_id"] = parentId
        return post("workspaces/${fixture.workspace.id}/documents", body, fixture.account.accessToken)
    }

    /**
     * Writes a document body. The endpoint is guarded by the `updated_at` the
     * caller last saw (autosave conflict detection), so the freshly created
     * document has to be passed in rather than just its id.
     */
    fun setDocumentContent(fixture: Fixture, document: Document, content: Any) {
        val req = Request.Builder()
            .url(apiUrl + "documents/${document.id}/content")
            .patch(
                gson.toJson(mapOf("content" to content, "updated_at" to document.updatedAt))
                    .toRequestBody(json),
            )
            .header("Authorization", "Bearer ${fixture.account.accessToken}")
            .build()
        http.newCall(req).execute().use { resp ->
            check(resp.isSuccessful) {
                "e2e seed document content ${document.id} failed: HTTP ${resp.code} ${resp.body.string()}"
            }
        }
    }

    // ── plumbing ───────────────────────────────────────────────────────────

    private inline fun <reified T> post(path: String, body: Any, token: String? = null): T {
        val req = Request.Builder()
            .url(apiUrl + path)
            .post(gson.toJson(body).toRequestBody(json))
            .apply { if (!token.isNullOrBlank()) header("Authorization", "Bearer $token") }
            .build()
        return execute(req, path)
    }

    private inline fun <reified T> patch(path: String, body: Any, token: String): T {
        val req = Request.Builder()
            .url(apiUrl + path)
            .patch(gson.toJson(body).toRequestBody(json))
            .header("Authorization", "Bearer $token")
            .build()
        return execute(req, path)
    }

    private inline fun <reified T> get(path: String, token: String): T {
        val req = Request.Builder()
            .url(apiUrl + path)
            .header("Authorization", "Bearer $token")
            .build()
        return execute(req, path)
    }

    private inline fun <reified T> getList(path: String, token: String): List<T> {
        val req = Request.Builder()
            .url(apiUrl + path)
            .apply { header("Authorization", "Bearer $token") }
            .build()
        val payload = raw(req, path)
        // The Go backend serialises an empty slice as `null`.
        val type = com.google.gson.reflect.TypeToken.getParameterized(
            List::class.java,
            T::class.java,
        ).type
        return gson.fromJson<List<T>>(payload, type) ?: emptyList()
    }

    private inline fun <reified T> execute(req: Request, path: String): T =
        gson.fromJson(raw(req, path), T::class.java)
            ?: error("e2e seed $path returned an empty body")

    private fun raw(req: Request, path: String): String =
        http.newCall(req).execute().use { resp ->
            val payload = resp.body.string()
            check(resp.isSuccessful) { "e2e seed ${req.method} $path failed: HTTP ${resp.code} $payload" }
            payload
        }
}
