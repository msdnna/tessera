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
import java.net.URLDecoder
import website.msdnna.tessera.reminders.ReminderNotifications
import website.msdnna.tessera.ui.AppRoot
import website.msdnna.tessera.ui.OAuthResult

class MainActivity : ComponentActivity() {
    // A task id to open on launch, set from a reminder-notification tap. Read
    // (and cleared) by AppRoot; updated on onNewIntent when already running.
    private val pendingOpenTaskId = mutableStateOf<String?>(null)

    // Session handed back by the OAuth deep link (tessera://oauth/callback#…).
    // Consumed by AppRoot, which persists the tokens and advances past auth.
    private val pendingOAuth = mutableStateOf<OAuthResult?>(null)

    private val notificationPermission =
        registerForActivityResult(ActivityResultContracts.RequestPermission()) { /* best-effort */ }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        pendingOpenTaskId.value = intent?.getStringExtra(ReminderNotifications.EXTRA_OPEN_TASK_ID)
        parseOAuth(intent)?.let { pendingOAuth.value = it }
        requestNotificationPermission()
        setContent {
            AppRoot(
                openTaskId = pendingOpenTaskId.value,
                onOpenTaskHandled = { pendingOpenTaskId.value = null },
                oauthResult = pendingOAuth.value,
                onOAuthHandled = { pendingOAuth.value = null },
            )
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        intent.getStringExtra(ReminderNotifications.EXTRA_OPEN_TASK_ID)?.let {
            pendingOpenTaskId.value = it
        }
        parseOAuth(intent)?.let { pendingOAuth.value = it }
    }

    /** Extracts the session (or error) from a tessera://oauth/callback deep link.
     *  The payload rides in the URL fragment so it never hits the server logs. */
    private fun parseOAuth(intent: Intent?): OAuthResult? {
        val data = intent?.data ?: return null
        if (data.scheme != "tessera" || data.host != "oauth") return null
        val frag = data.encodedFragment ?: return OAuthResult.Error("no_data")
        val params = frag.split("&").mapNotNull { part ->
            val i = part.indexOf('=')
            if (i <= 0) {
                null
            } else {
                runCatching {
                    URLDecoder.decode(part.substring(0, i), "UTF-8") to
                        URLDecoder.decode(part.substring(i + 1), "UTF-8")
                }.getOrNull()
            }
        }.toMap()
        params["oauth_error"]?.let { return OAuthResult.Error(it) }
        val access = params["access_token"].orEmpty()
        val refresh = params["refresh_token"].orEmpty()
        return if (access.isNotBlank() && refresh.isNotBlank()) {
            OAuthResult.Success(access, refresh)
        } else {
            OAuthResult.Error("no_tokens")
        }
    }

    private fun requestNotificationPermission() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return
        val granted = ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) ==
            PackageManager.PERMISSION_GRANTED
        if (!granted) notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
    }
}
