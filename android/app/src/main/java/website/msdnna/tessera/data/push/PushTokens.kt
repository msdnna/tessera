package website.msdnna.tessera.data.push

import android.content.Context
import android.os.Build
import com.google.firebase.Firebase
import com.google.firebase.messaging.messaging
import kotlinx.coroutines.flow.first
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.repository.NotificationRepository

/**
 * The FCM registration token: asking for one, persisting it, and telling the
 * backend about it.
 *
 * Firebase hands the token out asynchronously — `register()` only kicks the
 * process off, and the token itself arrives later in
 * [TesseraMessagingService.onRegistered], possibly while the app is closed. So
 * the token is cached in DataStore and every app start re-registers the device
 * with whatever we hold, which also covers a token that rotated in between.
 *
 * Everything here is best-effort by design. A build without google-services.json,
 * a phone with no Play Services, or a user who hasn't logged in yet all end up
 * with no token — and the device keeps working the way it did before background
 * push existed, over the live socket while the app is open.
 */
object PushTokens {
    /**
     * Kicks off (or refreshes) FCM registration and returns the token we already
     * hold, if any. Never throws: without a Firebase config there is no Firebase
     * app to talk to at all.
     */
    suspend fun ensureRegistered(): String {
        runCatching { Firebase.messaging.register() }
        return runCatching { AppContainer.prefs.fcmToken.first() }.getOrDefault("")
    }

    /**
     * Persists a freshly issued token and registers the device right away when a
     * session is already stored. Called from the messaging service, where the app
     * may be closed and nothing is primed — so the session is restored from
     * DataStore first, the same way the reminder sync does it. With no session
     * yet, the next app start registers the stored token.
     */
    suspend fun store(context: Context, token: String) {
        runCatching {
            val prefs = AppContainer.prefs
            prefs.setFcmToken(token)

            val auth = prefs.authToken.first()
            if (auth.isBlank()) return
            AppContainer.serverUrl = prefs.serverUrl.first()
            if (RetrofitClient.authToken.isBlank()) RetrofitClient.authToken = auth
            if (RetrofitClient.refreshToken.isBlank()) RetrofitClient.refreshToken = prefs.refreshToken.first()

            val deviceId = prefs.ensureDeviceId()
            if (deviceId.isBlank()) return
            NotificationRepository().registerDevice(deviceId, Build.MODEL ?: "Android", token)
        }
    }

    /** Forgets the cached token after FCM unregisters this install. The server
     *  drops its copy on its own once a send comes back UNREGISTERED. */
    suspend fun forget() {
        runCatching { AppContainer.prefs.setFcmToken("") }
    }
}
