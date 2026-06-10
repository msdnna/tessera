package website.msdnna.tessera.reminders

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import website.msdnna.tessera.MainActivity
import website.msdnna.tessera.R

/**
 * Fires when a reminder alarm goes off (scheduled by [ReminderScheduler]) and
 * posts a notification. Tapping it opens the app, deep-linking to the linked
 * task when the reminder had one.
 */
class ReminderReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val id = intent.getStringExtra(ReminderNotifications.EXTRA_REMINDER_ID) ?: return
        val message = intent.getStringExtra(ReminderNotifications.EXTRA_MESSAGE).orEmpty()
        val taskId = intent.getStringExtra(ReminderNotifications.EXTRA_TASK_ID)

        ReminderNotifications.ensureChannel(context)

        val launch = Intent(context, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
            if (!taskId.isNullOrBlank()) putExtra(ReminderNotifications.EXTRA_OPEN_TASK_ID, taskId)
        }
        val contentIntent = PendingIntent.getActivity(
            context,
            id.hashCode(),
            launch,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )

        val notification = NotificationCompat.Builder(context, ReminderNotifications.CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_stat_reminder)
            .setContentTitle("Напоминание")
            .setContentText(message.ifBlank { "Пора заняться задачей" })
            .setStyle(NotificationCompat.BigTextStyle().bigText(message.ifBlank { "Пора заняться задачей" }))
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setCategory(NotificationCompat.CATEGORY_REMINDER)
            .setAutoCancel(true)
            .setContentIntent(contentIntent)
            .build()

        runCatching {
            NotificationManagerCompat.from(context).notify(id.hashCode(), notification)
        }
    }
}
