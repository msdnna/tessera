package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * A personal reminder — mirrors the backend `Reminder` (`GET /reminders`).
 * [remindAt] is a full ISO-8601 instant (UTC `…Z`); unlike the web (storage
 * only), the Android client delivers these locally via [website.msdnna.tessera
 * .reminders.ReminderScheduler]. [taskId] optionally links it to a task.
 */
data class Reminder(
    @SerializedName("id") val id: String = "",
    @SerializedName("user_id") val userId: String = "",
    @SerializedName("task_id") val taskId: String? = null,
    @SerializedName("remind_at") val remindAt: String = "",
    @SerializedName("message") val message: String = "",
    @SerializedName("done") val done: Boolean = false,
    @SerializedName("created_at") val createdAt: String = "",
)

data class CreateReminderRequest(
    @SerializedName("remind_at") val remindAt: String,
    @SerializedName("message") val message: String,
    @SerializedName("task_id") val taskId: String? = null,
)

data class UpdateReminderRequest(
    @SerializedName("remind_at") val remindAt: String,
    @SerializedName("message") val message: String,
    @SerializedName("done") val done: Boolean,
)
