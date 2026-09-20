package website.msdnna.tessera.reminders

import android.app.AlarmManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import androidx.core.app.NotificationManagerCompat
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Reminder
import website.msdnna.tessera.util.parseInstantMillis

/**
 * Local delivery for reminders — the Android-specific half the web deferred.
 * Each not-done, future reminder gets an exact `AlarmManager` alarm that fires
 * [ReminderReceiver], which posts a notification. The server has no scheduler
 * or push, so this is the whole delivery path: we (re)schedule from the live
 * reminder list whenever it loads, and after reboot via [BootReceiver].
 */
object ReminderScheduler {
    /** Schedules (or replaces) the alarm for one reminder. Past / done ones are skipped. */
    fun schedule(context: Context, reminder: Reminder) {
        if (reminder.done) return
        val triggerAt = parseInstantMillis(reminder.remindAt) ?: return
        if (triggerAt <= System.currentTimeMillis()) return
        val alarm = context.getSystemService(AlarmManager::class.java) ?: return
        val pending = pendingIntent(context, reminder, create = true) ?: return

        // Exact when the OS allows it (clock-accurate delivery); otherwise an
        // idle-safe inexact alarm — still delivered, just not to the minute.
        val canExact = Build.VERSION.SDK_INT < Build.VERSION_CODES.S || alarm.canScheduleExactAlarms()
        if (canExact) {
            alarm.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, triggerAt, pending)
        } else {
            alarm.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, triggerAt, pending)
        }
    }

    /** Cancels the alarm for a reminder id (e.g. on delete / mark-done). */
    fun cancel(context: Context, reminderId: String) {
        val alarm = context.getSystemService(AlarmManager::class.java) ?: return
        val intent = Intent(context, ReminderReceiver::class.java).apply { action = actionFor(reminderId) }
        val pending = PendingIntent.getBroadcast(
            context,
            requestCode(reminderId),
            intent,
            PendingIntent.FLAG_NO_CREATE or PendingIntent.FLAG_IMMUTABLE,
        )
        if (pending != null) {
            alarm.cancel(pending)
            pending.cancel()
        }
    }

    /**
     * Replaces all scheduled alarms with the current reminder set: cancels every
     * known id, then schedules the future not-done ones. [previousIds] lets a
     * resync drop alarms for reminders that no longer exist.
     */
    fun rescheduleAll(context: Context, reminders: List<Reminder>, previousIds: Set<String> = emptySet()) {
        (previousIds + reminders.map { it.id }).forEach { cancel(context, it) }
        reminders.forEach { schedule(context, it) }
    }

    /**
     * То же, но «что было заведено раньше» берётся из DataStore, а не из памяти
     * вызывающего (#2932).
     *
     * Будильник живёт в системе, а приложение — нет: процесс перезапускается,
     * экран напоминаний пересоздаётся, и набор, лежавший полем ViewModel, каждый
     * раз начинался с пустого. Отменялось поэтому только то, что попало в ту же
     * загрузку, — напоминание, удалённое с другого устройства, всё равно звонило
     * в свой час.
     */
    suspend fun sync(context: Context, reminders: List<Reminder>) {
        val previous = AppContainer.prefs.scheduledReminderIds()
        rescheduleAll(context, reminders, previous)
        AppContainer.prefs.setScheduledReminders(reminders)
    }

    /**
     * Взводит будильники по последнему сохранённому списку, не ходя в сеть (#2932).
     *
     * Перезагрузка стирает будильники — их заводят заново из [BootReceiver], а тот
     * до сих пор начинал с запроса списка у сервера. Но `BOOT_COMPLETED` приходит
     * раньше, чем поднимается сеть, и на телефоне без связи запрос просто падал:
     * `runCatching` внутри [ReminderSync] это глотал, и «будильники остались как
     * были» после ребута означало «не осталось ни одного». Напоминание молчало до
     * следующего открытия экрана — то есть ровно тогда, когда человек уже и так
     * смотрит в приложение.
     *
     * Ничего не отменяет: отменять после ребута нечего, а повторный [schedule]
     * идёт под `FLAG_UPDATE_CURRENT` и просто заменяет уже заведённый будильник —
     * поэтому вызов безопасен и перед удачным походом в сеть, и после него.
     */
    suspend fun rearmFromCache(context: Context) {
        AppContainer.prefs.scheduledReminders().forEach { schedule(context, it) }
    }

    /**
     * Снимает всё, что эта сессия оставила на устройстве: будильники и уже
     * показанные по ним уведомления (#2932).
     *
     * Вызывается на выходе из аккаунта. Локальный будильник — единственный путь
     * доставки напоминания, а его текст лежит прямо в `PendingIntent`, то есть
     * переживает и токен, и профиль: без этой уборки личные напоминания
     * продолжали всплывать на экране уже вышедшего человека, а на общем
     * устройстве — у следующего вошедшего.
     */
    suspend fun cancelAllScheduled(context: Context) {
        val ids = AppContainer.prefs.scheduledReminderIds()
        val manager = NotificationManagerCompat.from(context)
        ids.forEach { id ->
            cancel(context, id)
            // Сработавший будильник уже нарисовал уведомление — оно висит в шторке
            // со своим текстом, и отмена будильника его не убирает.
            runCatching { manager.cancel(id.hashCode()) }
        }
        AppContainer.prefs.setScheduledReminders(emptyList())
    }

    private fun pendingIntent(context: Context, reminder: Reminder, create: Boolean): PendingIntent? {
        val intent = Intent(context, ReminderReceiver::class.java).apply {
            action = actionFor(reminder.id)
            putExtra(ReminderNotifications.EXTRA_REMINDER_ID, reminder.id)
            putExtra(ReminderNotifications.EXTRA_MESSAGE, reminder.message)
            putExtra(ReminderNotifications.EXTRA_TASK_ID, reminder.taskId)
        }
        val flags = (if (create) PendingIntent.FLAG_UPDATE_CURRENT else PendingIntent.FLAG_NO_CREATE) or
            PendingIntent.FLAG_IMMUTABLE
        return PendingIntent.getBroadcast(context, requestCode(reminder.id), intent, flags)
    }

    private fun actionFor(id: String) = "website.msdnna.tessera.REMINDER_$id"

    // A stable per-id request code so UPDATE_CURRENT replaces the right alarm.
    private fun requestCode(id: String) = id.hashCode()
}
