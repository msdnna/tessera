package website.msdnna.tessera.ui.viewmodels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.ChannelRequest
import website.msdnna.tessera.data.model.NotificationChannel
import website.msdnna.tessera.data.model.NotificationPrefs
import website.msdnna.tessera.data.model.NotificationRoute
import website.msdnna.tessera.data.model.RouteRequest
import website.msdnna.tessera.data.model.Workspace
import website.msdnna.tessera.data.repository.NotificationSettingsRepository
import website.msdnna.tessera.util.errorMessage

data class NotifSettingsUiState(
    val loading: Boolean = true,
    val error: String? = null,
    val message: String? = null,
    val channels: List<NotificationChannel> = emptyList(),
    val routes: List<NotificationRoute> = emptyList(),
    val prefs: NotificationPrefs = NotificationPrefs(),
    val workspaces: List<Workspace> = emptyList(),
    val saving: Boolean = false,
    val testingId: String? = null,
)

/** Owns the notification-settings screen: delivery channels, routing rules and
 *  per-user scheduling prefs. Mirrors the web `NotificationSettings.vue`. */
class NotificationSettingsViewModel : ViewModel() {
    private val repo = NotificationSettingsRepository()
    private val _state = MutableStateFlow(NotifSettingsUiState())
    val state: StateFlow<NotifSettingsUiState> = _state.asStateFlow()

    fun loadAll() {
        viewModelScope.launch {
            _state.update { it.copy(loading = true, error = null) }
            try {
                val channels = repo.channels()
                val routes = repo.routes()
                val prefs = repo.prefs()
                val workspaces = runCatching { repo.workspaces() }.getOrDefault(emptyList())
                _state.update {
                    it.copy(loading = false, channels = channels, routes = routes, prefs = prefs, workspaces = workspaces)
                }
            } catch (e: Exception) {
                _state.update { it.copy(loading = false, error = errorMessage(e)) }
            }
        }
    }

    private fun reload() {
        viewModelScope.launch {
            runCatching {
                val channels = repo.channels()
                val routes = repo.routes()
                _state.update { it.copy(channels = channels, routes = routes) }
            }
        }
    }

    fun saveChannel(existingId: String?, req: ChannelRequest, onDone: () -> Unit = {}) {
        viewModelScope.launch {
            _state.update { it.copy(saving = true, error = null) }
            try {
                if (existingId != null) repo.updateChannel(existingId, req) else repo.createChannel(req)
                _state.update { it.copy(saving = false) }
                reload()
                onDone()
            } catch (e: Exception) {
                _state.update { it.copy(saving = false, error = errorMessage(e)) }
            }
        }
    }

    fun toggleChannel(c: NotificationChannel) {
        saveChannel(
            c.id,
            ChannelRequest(c.type, c.label, c.config ?: emptyMap(), emptyMap(), c.template, !c.enabled),
        )
    }

    fun deleteChannel(id: String) {
        viewModelScope.launch {
            runCatching { repo.deleteChannel(id) }
            reload()
        }
    }

    fun testChannel(id: String) {
        viewModelScope.launch {
            _state.update { it.copy(testingId = id, error = null, message = null) }
            try {
                val r = repo.testChannel(id)
                _state.update {
                    it.copy(testingId = null, message = r.warning ?: "Тест отправлен")
                }
                reload()
            } catch (e: Exception) {
                _state.update { it.copy(testingId = null, error = errorMessage(e)) }
            }
        }
    }

    fun saveRoute(existingId: String?, req: RouteRequest, onDone: () -> Unit = {}) {
        viewModelScope.launch {
            _state.update { it.copy(saving = true, error = null) }
            try {
                if (existingId != null) repo.updateRoute(existingId, req) else repo.createRoute(req)
                _state.update { it.copy(saving = false) }
                reload()
                onDone()
            } catch (e: Exception) {
                _state.update { it.copy(saving = false, error = errorMessage(e)) }
            }
        }
    }

    fun toggleRoute(r: NotificationRoute) {
        saveRoute(r.id, RouteRequest(r.matcher, r.channelIds, r.options, !r.enabled, r.position))
    }

    fun deleteRoute(id: String) {
        viewModelScope.launch {
            runCatching { repo.deleteRoute(id) }
            reload()
        }
    }

    fun savePrefs(p: NotificationPrefs) {
        viewModelScope.launch {
            _state.update { it.copy(saving = true, error = null) }
            try {
                val saved = repo.savePrefs(p)
                _state.update { it.copy(saving = false, prefs = saved, message = "Сохранено") }
            } catch (e: Exception) {
                _state.update { it.copy(saving = false, error = errorMessage(e)) }
            }
        }
    }

    fun clearMessage() = _state.update { it.copy(message = null, error = null) }
}
