package website.msdnna.tessera.reminders

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

/**
 * Reschedules reminder alarms after a reboot or app update — `AlarmManager`
 * alarms don't survive either. Re-fetches the reminder list (priming the
 * session from DataStore) and re-arms the future ones.
 */
class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        when (intent.action) {
            Intent.ACTION_BOOT_COMPLETED,
            Intent.ACTION_MY_PACKAGE_REPLACED,
            -> {
                val pending = goAsync()
                val appContext = context.applicationContext
                CoroutineScope(Dispatchers.IO).launch {
                    try {
                        ReminderSync.sync(appContext)
                    } finally {
                        pending.finish()
                    }
                }
            }
        }
    }
}
