package website.msdnna.tessera.reminders

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import kotlinx.coroutines.launch
import website.msdnna.tessera.MainActivity
import website.msdnna.tessera.R
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.util.withLanguage

/**
 * Fires when a reminder alarm goes off (scheduled by [ReminderScheduler]) and
 * posts a notification. Tapping it opens the app, deep-linking to the linked
 * task when the reminder had one.
 */
class ReminderReceiver : BroadcastReceiver() {
    /**
     * Язык интерфейса лежит в DataStore, а `onReceive` синхронный — читаем его в
     * корутине под [goAsync], иначе будильник, поднявший процесс после перезагрузки,
     * успел бы нарисовать уведомление раньше, чем язык профиля вообще прочитан.
     */
    override fun onReceive(context: Context, intent: Intent) {
        val id = intent.getStringExtra(ReminderNotifications.EXTRA_REMINDER_ID) ?: return
        val message = intent.getStringExtra(ReminderNotifications.EXTRA_MESSAGE).orEmpty()
        val taskId = intent.getStringExtra(ReminderNotifications.EXTRA_TASK_ID)

        val pending = goAsync()
        AppContainer.appScope.launch {
            try {
                post(context, AppContainer.language(), id, message, taskId)
            } finally {
                pending.finish()
            }
        }
    }

    private fun post(context: Context, language: String, id: String, message: String, taskId: String?) {
        ReminderNotifications.ensureChannel(context, language)
        val localized = context.withLanguage(language)
        val body = message.ifBlank { localized.getString(R.string.reminder_push_default_body) }

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

        val notification = NotificationCompat.Builder(localized, ReminderNotifications.CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_stat_reminder)
            .setContentTitle(localized.getString(R.string.reminder_push_title))
            .setContentText(body)
            .setStyle(NotificationCompat.BigTextStyle().bigText(body))
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
