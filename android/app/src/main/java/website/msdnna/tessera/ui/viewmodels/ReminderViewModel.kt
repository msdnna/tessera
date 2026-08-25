package website.msdnna.tessera.ui.viewmodels

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.Reminder
import website.msdnna.tessera.data.repository.ReminderRepository
import website.msdnna.tessera.reminders.ReminderScheduler
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.errorMessage
import website.msdnna.tessera.util.parseInstantMillis

data class ReminderUiState(
    val loading: Boolean = true,
    val error: UiText? = null,
    val items: List<Reminder> = emptyList(),
)

/**
 * Personal reminders (web `RemindersView`) plus the Android-only delivery:
 * every load re-arms local alarms via [ReminderScheduler], so the on-device
 * notifications always reflect the latest list (including reminders created on
 * the web). An [AndroidViewModel] for the [Application] context the scheduler
 * needs.
 */
class ReminderViewModel(app: Application) : AndroidViewModel(app) {
    private val repo = ReminderRepository()
    private val _state = MutableStateFlow(ReminderUiState())
    val state: StateFlow<ReminderUiState> = _state.asStateFlow()

    // Ids we last scheduled, so a reload can cancel alarms for deleted reminders.
    private var scheduledIds: Set<String> = emptySet()

    init {
        load()
    }

    fun load() = launchCatching {
        val items = repo.list().sortedBy { parseInstantMillis(it.remindAt) ?: Long.MAX_VALUE }
        _state.update { it.copy(loading = false, items = items) }
        resync(items)
    }

    fun create(remindAtIso: String, message: String) = launchCatching {
        repo.create(remindAtIso, message)
        reload()
    }

    fun toggleDone(reminder: Reminder) = launchCatching {
        repo.update(reminder, reminder.remindAt, reminder.message, !reminder.done)
        reload()
    }

    fun delete(reminder: Reminder) = launchCatching {
        repo.delete(reminder.id)
        ReminderScheduler.cancel(getApplication<android.app.Application>(), reminder.id)
        reload()
    }

    fun clearError() = _state.update { it.copy(error = null) }

    private suspend fun reload() {
        val items = repo.list().sortedBy { parseInstantMillis(it.remindAt) ?: Long.MAX_VALUE }
        _state.update { it.copy(items = items) }
        resync(items)
    }

    private fun resync(items: List<Reminder>) {
        ReminderScheduler.rescheduleAll(getApplication<android.app.Application>(), items, scheduledIds)
        scheduledIds = items.map { it.id }.toSet()
    }

    private fun launchCatching(block: suspend () -> Unit) {
        viewModelScope.launch {
            val result = runCatching { block() }
            result.exceptionOrNull()?.let { e ->
                _state.update { it.copy(loading = false, error = errorMessage(e)) }
            }
        }
    }
}
