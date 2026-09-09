package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * A conference — a scheduled call in a workspace (#2896 §1, backend #2864).
 *
 * A conference is a *reusable* room (#2879): leaving it empty pauses it back to
 * `scheduled` rather than ending it, and the next arrival brings it live again.
 * So [status] is the state of the current session, not a lifecycle — a standup
 * that ran yesterday is `scheduled` again this morning, not `ended`.
 *
 * One model for both the list and the single-conference call: `GET
 * /workspaces/:id/conferences` joins in [createdByName] and the two counters,
 * while `GET /conferences/:id` returns the bare row inside a wrapper. The joined
 * fields are therefore nullable/zero rather than absent — see [ConferenceDetail].
 */
data class Conference(
    @SerializedName("id") val id: String = "",
    @SerializedName("workspace_id") val workspaceId: String = "",
    @SerializedName("task_id") val taskId: String? = null,
    @SerializedName("created_by") val createdBy: String? = null,
    @SerializedName("title") val title: String = "",
    @SerializedName("description") val description: String = "",
    @SerializedName("scheduled_at") val scheduledAt: String? = null,
    @SerializedName("started_at") val startedAt: String? = null,
    @SerializedName("ended_at") val endedAt: String? = null,
    @SerializedName("status") val status: String = "",
    @SerializedName("recording_ttl_days") val recordingTtlDays: Int = 0,
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("updated_at") val updatedAt: String = "",
    // Joined in by the list query only.
    @SerializedName("created_by_name") val createdByName: String? = null,
    @SerializedName("participant_count") val participantCount: Int = 0,
    @SerializedName("active_count") val activeCount: Int = 0,
) {
    /** Somebody is in the room right now — the only state you *join*, not open. */
    val isLive: Boolean get() = status == ConferenceStatus.LIVE

    val isEnded: Boolean get() = status == ConferenceStatus.ENDED

    /**
     * Planned but idle. Not the same as "never used": a paused reusable room
     * reads as scheduled too, which is why [startedAt] is not consulted here.
     */
    val isScheduled: Boolean get() = status == ConferenceStatus.SCHEDULED
}

/** The `status` values the backend accepts as a list filter and returns on a row. */
object ConferenceStatus {
    const val SCHEDULED = "scheduled"
    const val LIVE = "live"
    const val ENDED = "ended"

    /** The filter tabs on the list screen; `null` is "all". */
    val FILTERS: List<String?> = listOf(null, LIVE, SCHEDULED, ENDED)
}

/**
 * A participant row. Invitation and attendance are the same row: [invitedAt] is
 * always set, [joinedAt]/[leftAt] are stamps of the current session.
 *
 * [role] is `host` or `member` and decides moderation only — permission to be in
 * the call at all is workspace membership.
 */
data class ConferenceParticipant(
    @SerializedName("conference_id") val conferenceId: String = "",
    @SerializedName("user_id") val userId: String = "",
    @SerializedName("role") val role: String = "",
    @SerializedName("invited_at") val invitedAt: String = "",
    @SerializedName("joined_at") val joinedAt: String? = null,
    @SerializedName("left_at") val leftAt: String? = null,
    @SerializedName("force_muted") val forceMuted: Boolean = false,
    // Joined in by the list query; the join/leave responses carry the raw row.
    @SerializedName("user_name") val userName: String? = null,
    @SerializedName("user_email") val userEmail: String? = null,
) {
    val isHost: Boolean get() = role == "host"

    /**
     * In the room right now. A rejoin clears `left_at` server-side, so the pair
     * is read together rather than trusting [joinedAt] alone — a row left over
     * from yesterday's session has both stamps set.
     */
    val isPresent: Boolean get() = joinedAt != null && leftAt == null
}

/** `GET /conferences/:id` — the conference plus its roster in one round trip. */
data class ConferenceDetail(
    @SerializedName("conference") val conference: Conference = Conference(),
    @SerializedName("participants") val participants: List<ConferenceParticipant> = emptyList(),
)

/** The 200 of join/leave: the (possibly re-started or paused) call and our own row. */
data class ConferenceMembership(
    @SerializedName("conference") val conference: Conference = Conference(),
    @SerializedName("participant") val participant: ConferenceParticipant = ConferenceParticipant(),
)

/**
 * `POST /workspaces/:id/conferences`.
 *
 * [scheduledAt] is RFC3339 or absent (start it whenever). [recordingTtlDays] is
 * absent to take the server default of 30; 0 means "keep forever" rather than
 * "expire immediately", and a negative value is refused with a 400.
 */
data class CreateConferenceRequest(
    @SerializedName("title") val title: String,
    @SerializedName("description") val description: String = "",
    @SerializedName("scheduled_at") val scheduledAt: String? = null,
    @SerializedName("task_id") val taskId: String? = null,
    @SerializedName("recording_ttl_days") val recordingTtlDays: Int? = null,
)

/**
 * `PATCH /conferences/:id` — the plan only; start and end have their own routes.
 *
 * Unlike the document PATCH this one is a **full replace**: the server binds
 * `title` as required, so a partial edit that omits it is a 400. Callers build
 * the request from the conference they are showing.
 */
data class UpdateConferenceRequest(
    @SerializedName("title") val title: String,
    @SerializedName("description") val description: String = "",
    @SerializedName("scheduled_at") val scheduledAt: String? = null,
    @SerializedName("task_id") val taskId: String? = null,
    @SerializedName("recording_ttl_days") val recordingTtlDays: Int? = null,
)

/** `POST /conferences/:id/invite`. Invitees must be members of the same workspace. */
data class InviteConferenceRequest(
    @SerializedName("user_ids") val userIds: List<String>,
    @SerializedName("role") val role: String = "member",
)

/**
 * `POST /conferences/:id/token` — a short-lived LiveKit warrant.
 *
 * [expiresIn] is seconds; the token is re-requested on a reconnect rather than
 * cached for the length of the meeting. A 503 here means the install has no SFU
 * configured, a 409 that the call has ended, and a 403 that we are serving a
 * kick cooldown — all three are ordinary answers, not crashes (§4 acts on them).
 */
data class ConferenceToken(
    @SerializedName("url") val url: String = "",
    @SerializedName("token") val token: String = "",
    @SerializedName("room") val room: String = "",
    @SerializedName("identity") val identity: String = "",
    @SerializedName("expires_in") val expiresIn: Int = 0,
)

/** The `status` values of a recording row (migration 0071's CHECK). */
object RecordingStatus {
    const val ACTIVE = "active"
    const val COMPLETED = "completed"
    const val FAILED = "failed"
}

/** One line of the in-call chat (#2873). */
data class ConferenceMessage(
    @SerializedName("id") val id: String = "",
    @SerializedName("user_id") val userId: String? = null,
    @SerializedName("user_name") val userName: String? = null,
    @SerializedName("body") val body: String = "",
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("attachments") val attachments: List<ConferenceAttachment> = emptyList(),
)

/** A file on a chat message. [isImage] is the server's own call on renderability. */
data class ConferenceAttachment(
    @SerializedName("id") val id: String = "",
    @SerializedName("message_id") val messageId: String = "",
    @SerializedName("filename") val filename: String = "",
    @SerializedName("type") val type: String = "",
    @SerializedName("size") val size: Long = 0,
    @SerializedName("is_image") val isImage: Boolean = false,
    @SerializedName("created_at") val createdAt: String = "",
)

/**
 * A page of chat, oldest first. Paging walks *backwards* from the cursor
 * (`before_at` + `before_id`), so [hasMore] means "there is older text", not
 * newer — new messages arrive over the room socket.
 */
data class ConferenceMessagePage(
    @SerializedName("messages") val messages: List<ConferenceMessage> = emptyList(),
    @SerializedName("has_more") val hasMore: Boolean = false,
)

/** `POST /conferences/:id/messages` with no files — the common case. */
data class PostConferenceMessageRequest(
    @SerializedName("body") val body: String,
)

/**
 * A server-side recording (#2877).
 *
 * Failed rows are listed too, on purpose: a recording somebody started and never
 * got is exactly what they come asking about. [status] is `active`, `completed`
 * or `failed` (the CHECK on the column), and [error] carries the reason for the
 * last of those.
 */
data class ConferenceRecording(
    @SerializedName("id") val id: String = "",
    @SerializedName("conference_id") val conferenceId: String = "",
    @SerializedName("status") val status: String = "",
    @SerializedName("error") val error: String = "",
    @SerializedName("file_name") val fileName: String = "",
    @SerializedName("size_bytes") val sizeBytes: Long = 0,
    @SerializedName("duration_sec") val durationSec: Int = 0,
    @SerializedName("started_at") val startedAt: String = "",
    @SerializedName("ended_at") val endedAt: String? = null,
    @SerializedName("expires_at") val expiresAt: String? = null,
    @SerializedName("started_by") val startedBy: String? = null,
    @SerializedName("started_by_name") val startedByName: String? = null,
) {
    /** Still being written — the row exists before the file does. */
    val isRunning: Boolean get() = status == RecordingStatus.ACTIVE

    /** Downloadable. A `failed` row is shown but has no bytes behind it. */
    val isReady: Boolean get() = status == RecordingStatus.COMPLETED

    val isFailed: Boolean get() = status == RecordingStatus.FAILED
}
