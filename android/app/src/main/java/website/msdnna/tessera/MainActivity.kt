package website.msdnna.tessera

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.mutableStateOf
import androidx.core.content.ContextCompat
import website.msdnna.tessera.reminders.ReminderNotifications
import website.msdnna.tessera.ui.AppRoot

class MainActivity : ComponentActivity() {
    // A task id to open on launch, set from a reminder-notification tap. Read
    // (and cleared) by AppRoot; updated on onNewIntent when already running.
    private val pendingOpenTaskId = mutableStateOf<String?>(null)

    private val notificationPermission =
        registerForActivityResult(ActivityResultContracts.RequestPermission()) { /* best-effort */ }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        pendingOpenTaskId.value = intent?.getStringExtra(ReminderNotifications.EXTRA_OPEN_TASK_ID)
        requestNotificationPermission()
        setContent {
            AppRoot(
                openTaskId = pendingOpenTaskId.value,
                onOpenTaskHandled = { pendingOpenTaskId.value = null },
            )
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        intent.getStringExtra(ReminderNotifications.EXTRA_OPEN_TASK_ID)?.let {
            pendingOpenTaskId.value = it
        }
    }

    private fun requestNotificationPermission() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return
        val granted = ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) ==
            PackageManager.PERMISSION_GRANTED
        if (!granted) notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
    }
}
