package website.msdnna.tessera.reminders

import android.content.Context
import kotlinx.coroutines.flow.first
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.repository.ReminderRepository

/**
 * Fetches the reminder list and reschedules all local alarms. Used outside the
 * UI (after reboot) where the session isn't primed yet, so it first restores
 * the server URL + tokens from DataStore. Best-effort: any failure (no session,
 * offline) leaves alarms untouched.
 */
object ReminderSync {
    suspend fun sync(context: Context) {
        runCatching {
            val prefs = AppContainer.prefs
            val token = prefs.authToken.first()
            if (token.isBlank()) return
            AppContainer.serverUrl = prefs.serverUrl.first()
            if (RetrofitClient.authToken.isBlank()) RetrofitClient.authToken = token
            if (RetrofitClient.refreshToken.isBlank()) RetrofitClient.refreshToken = prefs.refreshToken.first()

            val reminders = ReminderRepository().list()
            ReminderNotifications.ensureChannel(context)
            ReminderScheduler.rescheduleAll(context, reminders)
        }
    }
}
