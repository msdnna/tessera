package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Notification
import website.msdnna.tessera.data.model.RegisterDeviceRequest

/** Reads the notification feed and marks items read. */
class NotificationRepository {
    private val api get() = AppContainer.api()

    suspend fun list(): List<Notification> = api.notifications().orEmpty()
    suspend fun unreadCount(): Long = api.unreadCount().count
    suspend fun markRead(id: String) = api.markNotificationRead(id)
    suspend fun markAllRead() = api.markAllNotificationsRead()

    /** Registers this device as a routable "device" channel (idempotent).
     *  [fcmToken] is what makes background push reach it while the app is closed;
     *  blank means "no push here" and leaves any token the server already holds
     *  alone (this call also runs on starts where Play Services is unreachable). */
    suspend fun registerDevice(deviceId: String, label: String, fcmToken: String = "") =
        api.registerDevice(RegisterDeviceRequest(deviceId, label, "android", fcmToken))
}
