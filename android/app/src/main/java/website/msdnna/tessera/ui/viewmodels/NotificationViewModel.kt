package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.Notification
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

    init {
        ensureRealtime()
        load()
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
        reloadJob?.cancel()
        reloadJob = viewModelScope.launch {
            delay(REALTIME_DEBOUNCE_MS)
            load()
        }
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
