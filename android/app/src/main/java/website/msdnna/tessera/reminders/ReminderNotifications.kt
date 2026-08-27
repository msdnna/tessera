package website.msdnna.tessera.reminders

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.os.Build
import website.msdnna.tessera.R
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.util.withLanguage

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

    /** Creates the reminders channel (idempotent) on the profile's language.
     *  Safe to call on every launch; the language lives in DataStore, hence suspend. */
    suspend fun ensureChannel(context: Context) = ensureChannel(context, AppContainer.language())

    /** Пересоздание намеренное: имя и описание система у существующего канала
     *  обновляет, а важность и пользовательские настройки сохраняет. Выйди мы
     *  раньше по «канал уже есть» — подпись осталась бы на языке первого запуска
     *  даже после переключения языка в профиле. */
    fun ensureChannel(context: Context, language: String) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(NotificationManager::class.java) ?: return
        val localized = context.withLanguage(language)
        val channel = NotificationChannel(
            CHANNEL_ID,
            localized.getString(R.string.notif_channel_reminders_name),
            NotificationManager.IMPORTANCE_HIGH,
        ).apply {
            description = localized.getString(R.string.notif_channel_reminders_desc)
        }
        manager.createNotificationChannel(channel)
    }
}
