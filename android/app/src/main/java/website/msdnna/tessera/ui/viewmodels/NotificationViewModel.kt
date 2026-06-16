package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.google.gson.JsonObject
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Notification
import website.msdnna.tessera.data.realtime.DevicePush
import website.msdnna.tessera.data.realtime.RealtimeClient
import website.msdnna.tessera.data.realtime.RealtimeEvent
import website.msdnna.tessera.data.repository.NotificationRepository
import website.msdnna.tessera.util.errorMessage

data class NotificationUiState(
    val loading: Boolean = true,
    val error: String? = null,
    val items: List<Notification> = emptyList(),
) {
    val unread: Int get() = items.count { it.isUnread }
}

/**
 * Owns the persistent-notification feed + bell badge (web `notifications`
 * store). Loads once, then a live socket reloads it on `notification` events
 * (the backend broadcasts one when a notification is created). Read state is
 * applied optimistically so the badge updates instantly.
 */
class NotificationViewModel(
    private val repo: NotificationRepository = NotificationRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(NotificationUiState())
    val state: StateFlow<NotificationUiState> = _state.asStateFlow()

    private var realtime: RealtimeClient? = null
    private var reloadJob: Job? = null

    // Device-targeted notifications to raise as a system notification (collected by
    // the UI, which has a Context). deviceId/meId are cached for the WS filter.
    private val _devicePush = MutableSharedFlow<DevicePush>(extraBufferCapacity = 8)
    val devicePush: SharedFlow<DevicePush> = _devicePush.asSharedFlow()

    @Volatile private var deviceId: String = ""

    @Volatile private var meId: String = ""

    init {
        ensureRealtime()
        load()
        viewModelScope.launch {
            meId = runCatching { AppContainer.prefs.user.first()?.id ?: "" }.getOrDefault("")
            deviceId = runCatching { AppContainer.prefs.ensureDeviceId() }.getOrDefault("")
            if (deviceId.isNotBlank()) {
                runCatching { repo.registerDevice(deviceId, android.os.Build.MODEL ?: "Android") }
            }
        }
    }

    fun load() = launchCatching {
        val items = repo.list()
        _state.update { it.copy(loading = false, items = items) }
    }

    fun markRead(notification: Notification) {
        if (notification.isUnread) {
            _state.update { s -> s.copy(items = s.items.map { if (it.id == notification.id) it.copy(readAt = NOW_SENTINEL) else it }) }
            launchCatching { repo.markRead(notification.id) }
        }
    }

    fun markAllRead() {
        _state.update { s -> s.copy(items = s.items.map { if (it.isUnread) it.copy(readAt = NOW_SENTINEL) else it }) }
        launchCatching { repo.markAllRead() }
    }

    private fun ensureRealtime() {
        if (realtime != null) return
        realtime = RealtimeClient(::onRealtimeEvent).also { it.connect() }
    }

    private fun onRealtimeEvent(ev: RealtimeEvent) {
        if (ev.type != "notification") return
        maybeDevicePush(ev.data)
        reloadJob?.cancel()
        reloadJob = viewModelScope.launch {
            delay(REALTIME_DEBOUNCE_MS)
            load()
        }
    }

    /** Raise a system notification when this device is among the event's targets
     *  and the notification is addressed to the current user. */
    private fun maybeDevicePush(data: JsonObject?) {
        if (data == null || deviceId.isBlank()) return
        runCatching {
            if (data.get("user_id")?.asString != meId) return
            val targets = data.getAsJsonArray("device_targets") ?: return
            val hit = (0 until targets.size()).any { i ->
                val el = targets.get(i)
                !el.isJsonNull && el.asString == deviceId
            }
            if (!hit) return
            val n = data.getAsJsonObject("notification") ?: return
            val kind = n.get("kind")?.takeUnless { it.isJsonNull }?.asString ?: ""
            val text = n.get("text")?.takeUnless { it.isJsonNull }?.asString ?: ""
            val taskId = n.get("task_id")?.takeUnless { it.isJsonNull }?.asString
            _devicePush.tryEmit(DevicePush(titleForKind(kind), text, taskId))
        }
    }

    private fun titleForKind(kind: String): String = when (kind) {
        "assigned" -> "Назначена задача"
        "comment" -> "Новый комментарий"
        "mention" -> "Вас упомянули"
        "updated" -> "Задача изменена"
        "moved" -> "Задача перемещена"
        "archived" -> "Задача архивирована"
        "due_soon" -> "Скоро дедлайн"
        "reminder" -> "Напоминание"
        else -> "Tessera"
    }

    override fun onCleared() {
        realtime?.close()
        realtime = null
    }

    private fun launchCatching(block: suspend () -> Unit) {
        viewModelScope.launch {
            val result = runCatching { block() }
            result.exceptionOrNull()?.let { e ->
                _state.update { it.copy(loading = false, error = errorMessage(e)) }
            }
        }
    }

    private companion object {
        const val REALTIME_DEBOUNCE_MS = 300L

        // A non-null placeholder for read_at so the item flips to "read" locally
        // before the server round-trip; the next reload replaces it with the
        // real timestamp.
        const val NOW_SENTINEL = "now"
    }
}
