package website.msdnna.tessera.data.conference

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import website.msdnna.tessera.MainActivity
import website.msdnna.tessera.R
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.util.normalizeLanguage
import website.msdnna.tessera.util.withLanguage

/**
 * Keeps a call alive while the app is not on screen (#2896 §4).
 *
 * Without it a conference is a background process holding a microphone, which
 * Android stops within a minute or so of the user switching apps — mid-sentence,
 * with nothing to explain it. The notification is not decoration either: a
 * foreground service is *how* the platform grants a backgrounded app continued
 * access to the microphone and camera, and the ongoing entry is the user's way
 * back to the call.
 *
 * The type is declared as microphone+camera together because the service starts
 * before we know which the user will turn on, and a service cannot widen its own
 * type later.
 */
class ConferenceCallService : android.app.Service() {
    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        ensureChannelIfMissing(this)
        // The type is only a concept from Q onwards; below it the argument is
        // ignored, so one call covers both rather than two branches that drift.
        val type = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE or ServiceInfo.FOREGROUND_SERVICE_TYPE_CAMERA
        } else {
            0
        }
        ServiceCompat.startForeground(this, NOTIFICATION_ID, notification(this), type)
        // Not sticky: a call the system had to kill is over, and reviving the
        // service without a room would show an ongoing notification for a
        // conference nobody is in.
        return START_NOT_STICKY
    }

    private fun notification(context: Context): Notification {
        val open = PendingIntent.getActivity(
            context,
            0,
            Intent(context, MainActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
            },
            PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(context, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_stat_reminder)
            .setContentTitle(context.getString(R.string.conf_call_ongoing))
            .setContentText(context.getString(R.string.conf_call_ongoing_hint))
            .setContentIntent(open)
            .setOngoing(true)
            .setCategory(NotificationCompat.CATEGORY_CALL)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
    }

    companion object {
        const val CHANNEL_ID = "conference"
        private const val NOTIFICATION_ID = 4201

        /**
         * The entry the SDK's own `mediaProjection` service posts while we are
         * sharing (#2896 §8). A second id, not this service's: the two run at the
         * same time and reusing the id would leave one of them replacing the
         * other's text — including the call entry that is the way back in.
         */
        const val SHARE_NOTIFICATION_ID = 4202

        /**
         * Start or stop the service as the call needs it.
         *
         * Starting is wrapped because Android refuses a microphone-typed
         * foreground service started from the background (`ForegroundService…
         * StartNotAllowedException`): a call joined from a notification tap can
         * land there. Losing the service is bad — the call runs only while the
         * app is visible — but crashing on the join is worse.
         */
        fun apply(context: Context, needed: Boolean) {
            val intent = Intent(context, ConferenceCallService::class.java)
            if (needed) {
                runCatching { ContextCompat.startForegroundService(context, intent) }
            } else {
                runCatching { context.stopService(intent) }
            }
        }

        /**
         * The shade entry for a screen being shared (#2896 §8).
         *
         * Handed to the LiveKit SDK, which owns the `mediaProjection` foreground
         * service the platform requires for a capture. Built here anyway, with
         * our channel and our words: while the display is being shared this app
         * is by definition not the thing on screen, and the shade is the only
         * surface that can say what is being captured — and that it is going to a
         * meeting rather than to a recording.
         */
        fun shareNotification(context: Context): Notification {
            ensureChannelIfMissing(context)
            val open = PendingIntent.getActivity(
                context,
                0,
                Intent(context, MainActivity::class.java).apply {
                    flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
                },
                PendingIntent.FLAG_IMMUTABLE,
            )
            return NotificationCompat.Builder(context, CHANNEL_ID)
                .setSmallIcon(R.drawable.ic_stat_reminder)
                .setContentTitle(context.getString(R.string.conf_share_ongoing))
                .setContentText(context.getString(R.string.conf_share_ongoing_hint))
                .setContentIntent(open)
                .setOngoing(true)
                .setCategory(NotificationCompat.CATEGORY_SERVICE)
                .setPriority(NotificationCompat.PRIORITY_LOW)
                .build()
        }

        /**
         * The call channel, on the profile's language.
         *
         * Its own channel rather than the reminders one: this entry is silent and
         * permanent for as long as the call lasts, and putting it on the
         * high-importance reminders channel would buzz the phone at the moment
         * somebody joins a meeting.
         *
         * Created when a call is joined rather than at startup. The label has to
         * come from the profile's language, which lives in DataStore and is
         * therefore only readable from a coroutine — doing that on every launch
         * means a background write racing whatever else touches channels, and it
         * buys nothing: nobody sees this channel until there is a call.
         */
        suspend fun ensureChannel(context: Context) = ensureChannel(context, AppContainer.language())

        fun ensureChannel(context: Context, language: String) {
            if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
            val manager = context.getSystemService(NotificationManager::class.java) ?: return
            val localized = context.withLanguage(language)
            val channel = NotificationChannel(
                CHANNEL_ID,
                localized.getString(R.string.notif_channel_conference_name),
                NotificationManager.IMPORTANCE_LOW,
            ).apply {
                description = localized.getString(R.string.notif_channel_conference_desc)
                setShowBadge(false)
            }
            manager.createNotificationChannel(channel)
        }

        /**
         * The safety net for a service that starts without the engine having
         * prepared a channel — a notification posted to a channel that does not
         * exist is dropped, and a foreground service without one is killed.
         *
         * Only fills a gap, never relabels: overwriting here would replace the
         * profile-language name the engine just wrote with the system one.
         */
        private fun ensureChannelIfMissing(context: Context) {
            if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
            val manager = context.getSystemService(NotificationManager::class.java) ?: return
            if (manager.getNotificationChannel(CHANNEL_ID) != null) return
            // The phone's language, since the profile's is not readable here.
            // Both fall back to Russian for anything unsupported.
            ensureChannel(context, normalizeLanguage(context.resources.configuration.locales[0]?.language))
        }
    }
}
