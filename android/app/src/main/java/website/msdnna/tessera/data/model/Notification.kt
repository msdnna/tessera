package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * A persistent notification — mirrors the backend `ListNotificationsRow`
 * (`GET /notifications`): the notification plus the joined task number/board
 * needed to open it. `read_at` null = unread (mirrors the web store).
 */
data class Notification(
    @SerializedName("id") val id: String = "",
    @SerializedName("user_id") val userId: String = "",
    @SerializedName("workspace_id") val workspaceId: String = "",
    @SerializedName("task_id") val taskId: String? = null,
    @SerializedName("actor_id") val actorId: String? = null,
    @SerializedName("kind") val kind: String = "",
    @SerializedName("text") val text: String = "",
    @SerializedName("read_at") val readAt: String? = null,
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("task_number") val taskNumber: Long? = null,
    @SerializedName("task_board_id") val taskBoardId: String? = null,
) {
    val isUnread: Boolean get() = readAt == null

    /** True when tapping should open a task (both ids present, as the web checks). */
    val opensTask: Boolean get() = !taskId.isNullOrBlank() && !taskBoardId.isNullOrBlank()
}

/** `{ "count": <n> }` from `GET /notifications/unread-count`. */
data class UnreadCount(@SerializedName("count") val count: Long = 0)
