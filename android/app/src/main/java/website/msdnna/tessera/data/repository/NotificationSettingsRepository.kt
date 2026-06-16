package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.ChannelRequest
import website.msdnna.tessera.data.model.NotificationChannel
import website.msdnna.tessera.data.model.NotificationPrefs
import website.msdnna.tessera.data.model.NotificationRoute
import website.msdnna.tessera.data.model.RegisterDeviceRequest
import website.msdnna.tessera.data.model.RouteRequest
import website.msdnna.tessera.data.model.TestChannelResult
import website.msdnna.tessera.data.model.Workspace

/** Notification router settings: delivery channels, routing rules, scheduling
 *  prefs, and this device's auto-registration. */
class NotificationSettingsRepository {
    private val api get() = AppContainer.api()

    suspend fun channels(): List<NotificationChannel> = api.notificationChannels().orEmpty()
    suspend fun createChannel(req: ChannelRequest): NotificationChannel = api.createNotificationChannel(req)
    suspend fun updateChannel(id: String, req: ChannelRequest): NotificationChannel =
        api.updateNotificationChannel(id, req)
    suspend fun deleteChannel(id: String) = api.deleteNotificationChannel(id)
    suspend fun testChannel(id: String): TestChannelResult = api.testNotificationChannel(id)
    suspend fun registerDevice(req: RegisterDeviceRequest): NotificationChannel = api.registerDevice(req)

    suspend fun routes(): List<NotificationRoute> = api.notificationRoutes().orEmpty()
    suspend fun createRoute(req: RouteRequest): NotificationRoute = api.createNotificationRoute(req)
    suspend fun updateRoute(id: String, req: RouteRequest): NotificationRoute = api.updateNotificationRoute(id, req)
    suspend fun deleteRoute(id: String) = api.deleteNotificationRoute(id)

    suspend fun prefs(): NotificationPrefs = api.notificationPrefs()
    suspend fun savePrefs(p: NotificationPrefs): NotificationPrefs = api.updateNotificationPrefs(p)

    suspend fun workspaces(): List<Workspace> = api.workspaces().orEmpty()
}
