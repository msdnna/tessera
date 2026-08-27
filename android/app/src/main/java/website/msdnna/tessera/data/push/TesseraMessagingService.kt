package website.msdnna.tessera.data.push

import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.realtime.DeviceNotifier

/**
 * Receives background push from FCM. This runs with no UI — the app may be
 * closed entirely — so it touches nothing but a Context: the payload is decoded
 * by [PushPayload] and handed straight to [DeviceNotifier], the same notifier the
 * live-socket path uses.
 *
 * The server sends data-only messages on purpose. A payload with a `notification`
 * block would be drawn by the system while the app is closed, which would skip
 * both the deep link into the task and the dedup against a copy the open app
 * already showed over the socket.
 */
class TesseraMessagingService : FirebaseMessagingService() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    override fun onMessageReceived(message: RemoteMessage) {
        val push = PushPayload.parse(message.data) ?: return
        // Показ ушёл в корутину: язык интерфейса живёт в DataStore, а канал и
        // заголовок обязаны быть на языке профиля, а не на локали телефона.
        scope.launch { DeviceNotifier.show(applicationContext, push) }
    }

    /**
     * Where the registration token actually arrives — both the first one and
     * every rotation (reinstall, cleared data, restore onto a new phone). Persist
     * it and re-register the device; without this, push silently stops arriving
     * after a rotation.
     */
    override fun onRegistered(token: String) {
        scope.launch { PushTokens.store(applicationContext, token) }
    }

    /** FCM dropped this install's registration — forget the stale token. */
    override fun onUnregistered(token: String) {
        scope.launch { PushTokens.forget() }
    }

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }
}
