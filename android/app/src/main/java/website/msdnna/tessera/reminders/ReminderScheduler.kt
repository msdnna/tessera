package website.msdnna.tessera.reminders

import android.app.AlarmManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
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
