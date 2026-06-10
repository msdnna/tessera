package website.msdnna.tessera.reminders

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.os.Build

/** IDs for the app's notification channels + intent extras shared across the
 *  reminder receivers. Centralised so the scheduler, receiver and Application
 *  agree on the same constants. */
object ReminderNotifications {
    const val CHANNEL_ID = "reminders"
    const val EXTRA_REMINDER_ID = "reminder_id"
    const val EXTRA_MESSAGE = "reminder_message"
    const val EXTRA_TASK_ID = "reminder_task_id"

    /** Deep-link extra read by MainActivity to open a task on launch. */
    const val EXTRA_OPEN_TASK_ID = "open_task_id"

    /** Creates the reminders channel (idempotent). Safe to call on every launch. */
    fun ensureChannel(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(NotificationManager::class.java) ?: return
        if (manager.getNotificationChannel(CHANNEL_ID) != null) return
        val channel = NotificationChannel(
            CHANNEL_ID,
            "Напоминания",
            NotificationManager.IMPORTANCE_HIGH,
        ).apply {
            description = "Срабатывания напоминаний по задачам"
        }
        manager.createNotificationChannel(channel)
    }
}
