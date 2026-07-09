package website.msdnna.tessera.data.realtime

import android.os.Handler
import android.os.Looper
import com.google.gson.Gson
import com.google.gson.annotations.SerializedName
import java.util.concurrent.TimeUnit
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import website.msdnna.tessera.data.api.RetrofitClient

/** A server→client realtime event. Mirrors `realtime.Event` (`data` ignored — the
 *  client reloads the affected slice rather than patching from the payload). */
data class RealtimeEvent(
    @SerializedName("scope") val scope: String = "",
    @SerializedName("type") val type: String = "",
    // The id of the user who triggered the event (absent for system/worker actions);
    // used to attribute board-activity toasts ("X создал…") and detect own actions.
    @SerializedName("actor") val actor: String = "",
    // The event payload — most consumers reload instead, but `notification` events
    // carry the recipient + device_targets used to raise a system notification.
    @SerializedName("data") val data: com.google.gson.JsonObject? = null,
)

/**
 * Opens the `/api/ws` WebSocket and invokes [onEvent] for every broadcast,
 * mirroring the web `useRealtime`. Auto-reconnects with a fixed backoff; the
 * caller filters by scope (workspace id). Online-first: there's no offline queue.
 */
class RealtimeClient(private val onEvent: (RealtimeEvent) -> Unit) {
    private val client = OkHttpClient.Builder()
        .pingInterval(20, TimeUnit.SECONDS)
        .retryOnConnectionFailure(true)
        .build()
    private val gson = Gson()
    private val main = Handler(Looper.getMainLooper())

    @Volatile private var ws: WebSocket? = null

    @Volatile private var closed = false

    fun connect() {
        if (closed || ws != null) return
        val url = wsUrl() ?: return
        val req = Request.Builder().url(url).build()
        ws = client.newWebSocket(
            req,
            object : WebSocketListener() {
                override fun onMessage(webSocket: WebSocket, text: String) {
                    val ev = runCatching { gson.fromJson(text, RealtimeEvent::class.java) }.getOrNull()
                    if (ev != null && ev.type.isNotBlank()) onEvent(ev)
                }

                override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                    ws = null
                    scheduleReconnect()
                }

                override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                    ws = null
                    scheduleReconnect()
                }
            },
        )
    }

    private fun scheduleReconnect() {
        if (closed) return
        main.postDelayed({ connect() }, RECONNECT_MS)
    }

    fun close() {
        closed = true
        ws?.close(NORMAL_CLOSURE, null)
        ws = null
    }

    /** Derives the ws(s)://…/api/ws URL from the active server root. */
    private fun wsUrl(): String? {
        val root = RetrofitClient.serverRoot.ifBlank { return null }
        val scheme = when {
            root.startsWith("https") -> "wss" + root.removePrefix("https")
            root.startsWith("http") -> "ws" + root.removePrefix("http")
            else -> return null
        }
        return "$scheme/api/ws"
    }

    private companion object {
        const val RECONNECT_MS = 3000L
        const val NORMAL_CLOSURE = 1000
    }
}
