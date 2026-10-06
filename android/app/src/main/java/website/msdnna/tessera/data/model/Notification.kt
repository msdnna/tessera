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
    @SerializedName("payload") val payload: NotificationPayload? = null,
    @SerializedName("read_at") val readAt: String? = null,
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("task_number") val taskNumber: Long? = null,
    @SerializedName("task_board_id") val taskBoardId: String? = null,
) {
    val isUnread: Boolean get() = readAt == null
}

/**
 * The facts of what happened, stored alongside the sentence the server composed
 * from them (`text`). The client words the sentence itself, in the reader's
 * language — see `util/NotificationText.kt`; the server only does it for
 * deliveries that leave the app (email/FCM/telegram), where no client is there
 * to do it.
 *
 * Every field is nullable on purpose: a payload carries only the keys its own
 * event needs, and rows written before migration 0065 carry no payload at all.
 */
data class NotificationPayload(
    @SerializedName("event") val event: String? = null,
    @SerializedName("actor") val actor: String? = null,
    @SerializedName("task_number") val taskNumber: Long? = null,
    @SerializedName("title") val title: String? = null,
    @SerializedName("excerpt") val excerpt: String? = null,
    @SerializedName("column") val column: String? = null,
    /** Key of a server-seeded column name (#2800); absent for one the user named. */
    @SerializedName("column_key") val columnKey: String? = null,
    @SerializedName("label") val label: String? = null,
    @SerializedName("reason") val reason: String? = null,
    /**
     * Id звонка, в который зовут (`conference_invite`). Такая строка не указывает
     * ни на какую задачу — `notifications.task_id` это внешний ключ на tasks, а
     * конференция не задача, — поэтому адрес едет здесь. См. `util/NotificationRoute.kt`.
     */
    @SerializedName("conference_id") val conferenceId: String? = null,
    /** A reminder's own text — the user's content, shown verbatim. */
    @SerializedName("message") val message: String? = null,
    @SerializedName("created") val created: Int? = null,
    @SerializedName("updated") val updated: Int? = null,
    @SerializedName("seconds") val seconds: Int? = null,
    @SerializedName("fields") val fields: List<String>? = null,
)

/** `{ "count": <n> }` from `GET /notifications/unread-count`. */
data class UnreadCount(@SerializedName("count") val count: Long = 0)
