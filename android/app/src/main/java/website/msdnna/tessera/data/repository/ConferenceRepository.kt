package website.msdnna.tessera.data.repository

import okhttp3.MediaType.Companion.toMediaTypeOrNull
import okhttp3.MultipartBody
import okhttp3.RequestBody.Companion.toRequestBody
import retrofit2.HttpException
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Conference
import website.msdnna.tessera.data.model.ConferenceDetail
import website.msdnna.tessera.data.model.ConferenceMembership
import website.msdnna.tessera.data.model.ConferenceMessage
import website.msdnna.tessera.data.model.ConferenceMessagePage
import website.msdnna.tessera.data.model.ConferenceParticipant
import website.msdnna.tessera.data.model.ConferenceRecording
import website.msdnna.tessera.data.model.ConferenceToken
import website.msdnna.tessera.data.model.CreateConferenceRequest
import website.msdnna.tessera.data.model.InviteConferenceRequest
import website.msdnna.tessera.data.model.PostConferenceMessageRequest
import website.msdnna.tessera.data.model.UpdateConferenceRequest

/**
 * Why a token request failed.
 *
 * All three refusals are ordinary states of a working install, not errors to
 * show as a crash: a server without LiveKit configured, a call that has been
 * ended, and a kick cooldown still running. Retrofit would raise each as the
 * same [HttpException], so the room screen (§4) could only tell them apart by
 * re-reading the status code — which is exactly the kind of check that gets
 * written once and then forgotten in the next branch.
 */
sealed interface ConfTokenResult {
    data class Granted(val token: ConferenceToken) : ConfTokenResult

    /** The install has no SFU — conferences degrade to "unavailable", not broken. */
    data object NotConfigured : ConfTokenResult

    /** The call is over. Rejoining means starting it again through /join. */
    data object Ended : ConfTokenResult

    /** Removed by a moderator; [retryAfter] is the RFC3339 end of the cooldown. */
    data class Kicked(val retryAfter: String) : ConfTokenResult
}

/**
 * Conferences (#2896 §1). Mirrors the web `conferences` API module.
 *
 * Thin on purpose: everything here is one call plus null-flattening, because the
 * interesting decisions (which page to hold, when to re-request a token, what a
 * paused room means for the UI) belong to the view models that own the state,
 * not to a layer they all share.
 */
class ConferenceRepository {
    private val api get() = AppContainer.api()

    // ── The meeting ───────────────────────────────────────────────────────────

    /** [status] is one of `scheduled`/`live`/`ended`; null lists everything. */
    suspend fun list(workspaceId: String, status: String? = null): List<Conference> =
        api.conferences(workspaceId, status).orEmpty()

    suspend fun listForTask(taskId: String): List<Conference> =
        api.taskConferences(taskId).orEmpty()

    suspend fun get(id: String): ConferenceDetail = api.conference(id)

    suspend fun create(
        workspaceId: String,
        title: String,
        description: String = "",
        scheduledAt: String? = null,
        taskId: String? = null,
        recordingTtlDays: Int? = null,
    ): Conference = api.createConference(
        workspaceId,
        CreateConferenceRequest(
            title = title,
            description = description,
            scheduledAt = scheduledAt,
            taskId = taskId,
            recordingTtlDays = recordingTtlDays,
        ),
    )

    /**
     * A full replace: the server requires `title`, so callers pass the whole plan
     * rather than the one field they touched.
     */
    suspend fun update(
        id: String,
        title: String,
        description: String = "",
        scheduledAt: String? = null,
        taskId: String? = null,
        recordingTtlDays: Int? = null,
    ): Conference = api.updateConference(
        id,
        UpdateConferenceRequest(
            title = title,
            description = description,
            scheduledAt = scheduledAt,
            taskId = taskId,
            recordingTtlDays = recordingTtlDays,
        ),
    )

    suspend fun delete(id: String) = api.deleteConference(id)

    // ── Attendance ────────────────────────────────────────────────────────────
    suspend fun join(id: String): ConferenceMembership = api.joinConference(id)

    suspend fun leave(id: String): ConferenceMembership = api.leaveConference(id)

    /** Moderators only — a plain member gets a 403 rather than ending the call. */
    suspend fun end(id: String): Conference = api.endConference(id)

    suspend fun invite(
        id: String,
        userIds: List<String>,
        role: String = "member",
    ): List<ConferenceParticipant> =
        api.inviteConference(id, InviteConferenceRequest(userIds, role)).orEmpty()

    suspend fun participants(id: String): List<ConferenceParticipant> =
        api.conferenceParticipants(id).orEmpty()

    /**
     * Asks for a LiveKit warrant, mapping the three expected refusals onto
     * [ConfTokenResult] instead of letting them surface as a generic HTTP error.
     */
    suspend fun token(id: String): ConfTokenResult = try {
        ConfTokenResult.Granted(api.conferenceToken(id))
    } catch (e: HttpException) {
        when (e.code()) {
            HTTP_UNAVAILABLE -> ConfTokenResult.NotConfigured
            HTTP_CONFLICT -> ConfTokenResult.Ended
            HTTP_FORBIDDEN -> ConfTokenResult.Kicked(retryAfterOf(e))
            else -> throw e
        }
    }

    // ── Chat ──────────────────────────────────────────────────────────────────

    /**
     * A page of chat, oldest first. [beforeAt]/[beforeId] travel together — the
     * server rejects half a cursor, because a timestamp alone would compare
     * against a NULL uuid and quietly return nothing.
     */
    suspend fun messages(
        id: String,
        beforeAt: String? = null,
        beforeId: String? = null,
        limit: Int? = null,
    ): ConferenceMessagePage {
        require((beforeAt == null) == (beforeId == null)) {
            "both cursor halves or neither: before_at=$beforeAt before_id=$beforeId"
        }
        return api.conferenceMessages(id, beforeAt, beforeId, limit)
    }

    suspend fun postMessage(id: String, body: String): ConferenceMessage =
        api.postConferenceMessage(id, PostConferenceMessageRequest(body))

    /**
     * A message with files. The body rides along as a form field rather than a
     * second request: an upload that lost its caption on a flaky link would
     * arrive as a bare file with no idea what it was for.
     */
    suspend fun postMessageWithFiles(
        id: String,
        body: String,
        files: List<MultipartBody.Part>,
    ): ConferenceMessage = api.postConferenceMessageWithFiles(
        id,
        body.toRequestBody(TEXT_PLAIN.toMediaTypeOrNull()),
        files,
    )

    suspend fun deleteMessage(messageId: String) = api.deleteConferenceMessage(messageId)

    /**
     * Streams a chat attachment into the cache, returning the file.
     *
     * A file rather than the bytes: the thing that opens it is the system viewer
     * behind our `FileProvider`, exactly as task attachments do it, and a
     * `ByteArray` would have to be written out anyway — after being held whole,
     * at up to 25 MB, on top of a running call.
     */
    suspend fun downloadAttachment(cacheDir: java.io.File, attachmentId: String, filename: String): java.io.File {
        val body = api.downloadConferenceAttachment(attachmentId)
        val dir = java.io.File(cacheDir, "conference-attachments").apply { mkdirs() }
        // The name comes from whoever uploaded it: a «../» in it would otherwise
        // write outside the directory we chose.
        val safe = filename.ifBlank { attachmentId }.replace(Regex("[/\\\\]"), "_")
        val out = java.io.File(dir, safe)
        body.byteStream().use { input -> out.outputStream().use { input.copyTo(it) } }
        return out
    }

    // ── Recording ─────────────────────────────────────────────────────────────

    /**
     * Starts a server-side recording. A 502 here means egress is not reachable —
     * the button reports that honestly rather than pretending it started.
     */
    suspend fun startRecording(id: String): ConferenceRecording = api.startConferenceRecording(id)

    suspend fun stopRecording(id: String) = api.stopConferenceRecording(id)

    /** Includes failed rows: a recording that never arrived is what people ask about. */
    suspend fun recordings(id: String): List<ConferenceRecording> =
        api.conferenceRecordings(id).orEmpty()

    /**
     * Streams a recording into the cache, returning the file.
     *
     * Same shape as [downloadAttachment] and for a sharper version of the same
     * reason: an mp4 of a meeting is tens of megabytes, and holding one whole in
     * a `ByteArray` to then write it out anyway is how a phone runs out of heap
     * on the way back from a call.
     */
    suspend fun downloadRecordingTo(
        cacheDir: java.io.File,
        recordingId: String,
        filename: String,
    ): java.io.File {
        val body = api.downloadConferenceRecording(recordingId)
        val dir = java.io.File(cacheDir, "conference-recordings").apply { mkdirs() }
        val safe = filename.ifBlank { recordingId }.replace(Regex("[/\\\\]"), "_")
        val out = java.io.File(dir, safe)
        body.byteStream().use { input -> out.outputStream().use { input.copyTo(it) } }
        return out
    }

    suspend fun deleteRecording(recordingId: String) = api.deleteConferenceRecording(recordingId)

    private companion object {
        const val HTTP_FORBIDDEN = 403
        const val HTTP_CONFLICT = 409
        const val HTTP_UNAVAILABLE = 503
        const val TEXT_PLAIN = "text/plain"

        /**
         * Pulls `retry_after` out of a 403 body. Empty when the field is missing:
         * the cooldown's exact end is a nicety, and failing to parse it must not
         * turn a "you were removed" message into a crash.
         */
        fun retryAfterOf(e: HttpException): String = runCatching {
            val raw = e.response()?.errorBody()?.string().orEmpty()
            com.google.gson.JsonParser.parseString(raw)
                .asJsonObject.get("retry_after")?.asString.orEmpty()
        }.getOrDefault("")
    }
}
