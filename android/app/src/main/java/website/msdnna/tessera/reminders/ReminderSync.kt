package website.msdnna.tessera.reminders

import android.content.Context
import kotlinx.coroutines.flow.first
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.repository.ReminderRepository

/**
 * Fetches the reminder list and reschedules all local alarms. Used outside the
 * UI (after reboot) where the session isn't primed yet, so it first restores
 * the server URL + tokens from DataStore.
 *
 * Сначала перевзводит будильники из кэша, и только потом идёт в сеть (#2932).
 * Порядок тут и есть суть: после перезагрузки будильников не осталось ни одного,
 * а `BOOT_COMPLETED` приходит раньше связи — начав с запроса, на телефоне без
 * сети мы не взводили ничего и молчали до следующего открытия экрана. Сетевая
 * половина по-прежнему best-effort: упала — остаётся то, что взведено по кэшу.
 */
object ReminderSync {
    suspend fun sync(context: Context) {
        runCatching {
            val prefs = AppContainer.prefs
            val token = prefs.authToken.first()
            if (token.isBlank()) return
            ReminderNotifications.ensureChannel(context)
            ReminderScheduler.rearmFromCache(context)
            AppContainer.serverUrl = prefs.serverUrl.first()
            if (RetrofitClient.authToken.isBlank()) RetrofitClient.authToken = token
            if (RetrofitClient.refreshToken.isBlank()) RetrofitClient.refreshToken = prefs.refreshToken.first()

            val reminders = ReminderRepository().list()
            ReminderScheduler.sync(context, reminders)
        }
    }
}
