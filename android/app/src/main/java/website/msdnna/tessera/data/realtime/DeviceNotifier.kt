package website.msdnna.tessera.data.realtime

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import website.msdnna.tessera.MainActivity
import website.msdnna.tessera.R
import website.msdnna.tessera.reminders.ReminderNotifications

/** A notification the server routed to this device's "device" channel — shown as
 *  a system notification while the app is connected. */
data class DevicePush(val title: String, val body: String, val taskId: String?)

/** Posts [DevicePush]es as system notifications (the device-channel delivery path
 *  when the app is open). Mirrors the reminder receiver's posting. */
object DeviceNotifier {
    private const val CHANNEL_ID = "notifications"

    fun show(context: Context, push: DevicePush) {
        ensureChannel(context)
        val launch = Intent(context, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
            if (!push.taskId.isNullOrBlank()) putExtra(ReminderNotifications.EXTRA_OPEN_TASK_ID, push.taskId)
        }
        val id = (push.taskId ?: push.body).hashCode()
        val pi = PendingIntent.getActivity(
            context, id, launch,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val notification = NotificationCompat.Builder(context, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_stat_reminder)
            .setContentTitle(push.title.ifBlank { "Tessera" })
            .setContentText(push.body)
            .setStyle(NotificationCompat.BigTextStyle().bigText(push.body))
            .setPriority(NotificationCompat.PRIORITY_DEFAULT)
            .setAutoCancel(true)
            .setContentIntent(pi)
            .build()
        runCatching { NotificationManagerCompat.from(context).notify(id, notification) }
    }

    private fun ensureChannel(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(NotificationManager::class.java) ?: return
        if (manager.getNotificationChannel(CHANNEL_ID) != null) return
        manager.createNotificationChannel(
            NotificationChannel(CHANNEL_ID, "Уведомления", NotificationManager.IMPORTANCE_DEFAULT).apply {
                description = "Уведомления о задачах, когда приложение открыто"
            },
        )
    }
}
