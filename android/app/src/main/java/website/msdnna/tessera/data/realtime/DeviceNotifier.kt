package website.msdnna.tessera.data.realtime

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import androidx.annotation.StringRes
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import website.msdnna.tessera.MainActivity
import website.msdnna.tessera.R
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.reminders.ReminderNotifications
import website.msdnna.tessera.util.withLanguage

/** A notification the server routed to this device's "device" channel — shown as
 *  a system notification. Reaches us over the live socket while the app is open
 *  and as an FCM push while it's closed; [id] is the notification's server-side
 *  UUID, which keys the system notification so the two paths collapse into one
 *  entry instead of showing the same thing twice. */
data class DevicePush(
    val title: String,
    val body: String,
    val taskId: String?,
    val id: String? = null,
    /** Заголовок ресурсом, а не текстом: пуш живёт от события до показа, и
     *  готовая строка застыла бы на языке, который стоял в момент события. */
    @StringRes val titleRes: Int? = null,
)

/** Posts [DevicePush]es as system notifications (the device-channel delivery path
 *  when the app is open). Mirrors the reminder receiver's posting. */
object DeviceNotifier {
    private const val CHANNEL_ID = "notifications"

    /** Строки берутся из профиля пользователя, а не из локали телефона, — язык
     *  лежит в DataStore, поэтому показ пуша стал suspend. */
    suspend fun show(context: Context, push: DevicePush) = show(context, push, AppContainer.language())

    fun show(context: Context, push: DevicePush, language: String) {
        val localized = context.withLanguage(language)
        ensureChannel(localized)
        val launch = Intent(context, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
            if (!push.taskId.isNullOrBlank()) putExtra(ReminderNotifications.EXTRA_OPEN_TASK_ID, push.taskId)
        }
        // Key on the notification's own id so the socket copy and the push copy
        // of the same notification reuse one system entry (notify() redraws it)
        // instead of stacking. Fall back to the old key when there's no id.
        val id = (push.id ?: push.taskId ?: push.body).hashCode()
        val pi = PendingIntent.getActivity(
            context, id, launch,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val title = push.titleRes?.let { localized.getString(it) } ?: push.title
        val notification = NotificationCompat.Builder(localized, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_stat_reminder)
            .setContentTitle(title.ifBlank { localized.getString(R.string.app_name) })
            .setContentText(push.body)
            .setStyle(NotificationCompat.BigTextStyle().bigText(push.body))
            .setPriority(NotificationCompat.PRIORITY_DEFAULT)
            .setAutoCancel(true)
            .setContentIntent(pi)
            .build()
        runCatching { NotificationManagerCompat.from(context).notify(id, notification) }
    }

    /** Канал создаётся заново на каждом показе намеренно: имя и описание у
     *  существующего канала система обновляет, а важность и настройки, которые
     *  пользователь мог поменять сам, оставляет. Ранний выход «канал уже есть»
     *  заморозил бы подпись на языке первого запуска. */
    private fun ensureChannel(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(NotificationManager::class.java) ?: return
        manager.createNotificationChannel(
            NotificationChannel(
                CHANNEL_ID,
                context.getString(R.string.notif_channel_device_name),
                NotificationManager.IMPORTANCE_DEFAULT,
            ).apply {
                description = context.getString(R.string.notif_channel_device_desc)
            },
        )
    }
}
