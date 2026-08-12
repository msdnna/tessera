package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/** A delivery channel (mirrors the backend `channelView`). `config` holds
 *  non-secret settings (address / chat_id / url / device_id …); the secret is
 *  never returned (only [hasSecret]). */
data class NotificationChannel(
    @SerializedName("id") val id: String = "",
    @SerializedName("type") val type: String = "",
    @SerializedName("label") val label: String = "",
    @SerializedName("config") val config: Map<String, String>? = null,
    @SerializedName("template") val template: String = "",
    @SerializedName("enabled") val enabled: Boolean = true,
    @SerializedName("verified") val verified: Boolean = false,
    @SerializedName("has_secret") val hasSecret: Boolean = false,
)

/** Create/update body. `secret` carries plaintext secret fields; omit/empty on
 *  update keeps the stored secret. `template` null on update keeps it. */
data class ChannelRequest(
    @SerializedName("type") val type: String,
    @SerializedName("label") val label: String,
    @SerializedName("config") val config: Map<String, String>,
    @SerializedName("secret") val secret: Map<String, String>,
    @SerializedName("template") val template: String? = null,
    @SerializedName("enabled") val enabled: Boolean,
)

data class TestChannelResult(
    @SerializedName("ok") val ok: Boolean = false,
    @SerializedName("warning") val warning: String? = null,
    @SerializedName("error") val error: String? = null,
)

data class TemplatePreviewRequest(
    @SerializedName("template") val template: String,
)

data class TemplatePreviewResult(
    @SerializedName("ok") val ok: Boolean = false,
    @SerializedName("text") val text: String? = null,
    @SerializedName("error") val error: String? = null,
)

data class RegisterDeviceRequest(
    @SerializedName("device_id") val deviceId: String,
    @SerializedName("label") val label: String,
    @SerializedName("platform") val platform: String,
    // Optional: the FCM registration token for background push. Omitted (blank)
    // when this build/device has none — the server then keeps whatever it holds.
    @SerializedName("fcm_token") val fcmToken: String = "",
)

/** A routing rule's match condition (empty = matches anything). */
data class RouteMatcher(
    @SerializedName("kinds") val kinds: List<String>? = null,
    @SerializedName("workspace_id") val workspaceId: String? = null,
)

data class RouteOptions(
    @SerializedName("mute") val mute: Boolean = false,
)

/** A routing rule (mirrors `routeView`). */
data class NotificationRoute(
    @SerializedName("id") val id: String = "",
    @SerializedName("position") val position: Double = 0.0,
    @SerializedName("matcher") val matcher: RouteMatcher = RouteMatcher(),
    @SerializedName("channel_ids") val channelIds: List<String> = emptyList(),
    @SerializedName("options") val options: RouteOptions = RouteOptions(),
    @SerializedName("enabled") val enabled: Boolean = true,
)

data class RouteRequest(
    @SerializedName("matcher") val matcher: RouteMatcher,
    @SerializedName("channel_ids") val channelIds: List<String>,
    @SerializedName("options") val options: RouteOptions,
    @SerializedName("enabled") val enabled: Boolean,
    @SerializedName("position") val position: Double? = null,
)

/** Per-user scheduling prefs (mirrors `prefsView`). */
data class NotificationPrefs(
    @SerializedName("due_enabled") val dueEnabled: Boolean = true,
    @SerializedName("due_lead_minutes") val dueLeadMinutes: Int = 60,
    @SerializedName("due_repeat_minutes") val dueRepeatMinutes: Int = 0,
    @SerializedName("reminder_enabled") val reminderEnabled: Boolean = true,
    @SerializedName("quiet_enabled") val quietEnabled: Boolean = false,
    @SerializedName("quiet_start_minutes") val quietStartMinutes: Int = 1320,
    @SerializedName("quiet_end_minutes") val quietEndMinutes: Int = 480,
    @SerializedName("quiet_tz") val quietTz: String = "",
    @SerializedName("digest_minutes") val digestMinutes: Int = 0,
)
