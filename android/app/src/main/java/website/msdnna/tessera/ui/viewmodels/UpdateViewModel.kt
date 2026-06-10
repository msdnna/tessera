package website.msdnna.tessera.ui.viewmodels

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import java.io.File
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.model.LatestRelease
import website.msdnna.tessera.update.UpdateRepository

/** Drives the in-app update *dialog*. [release] rides along so the dialog keeps
 *  its title/notes through every phase. Whether an update merely *exists* is
 *  tracked separately by [UpdateViewModel.available] (survives a dismiss). */
sealed interface UpdateState {
    data object Idle : UpdateState
    data class Available(val release: LatestRelease) : UpdateState
    data class Downloading(val progress: Float, val release: LatestRelease) : UpdateState
    data class Ready(val file: File, val release: LatestRelease) : UpdateState
    data class Failed(val message: String, val release: LatestRelease) : UpdateState
}

/**
 * Checks for a newer release on creation and on every foreground resume. A found
 * release is held in [available] regardless of the dialog — so "Позже" only hides
 * the dialog for the session; the sidebar still offers "Обновить" via [available]
 * and [startDownload]. Download streams to the cache; install hands off to the
 * system package installer.
 */
class UpdateViewModel(app: Application) : AndroidViewModel(app) {
    private val _state = MutableStateFlow<UpdateState>(UpdateState.Idle)
    val state: StateFlow<UpdateState> = _state.asStateFlow()

    /** The newest available release, or null if up to date. Independent of the
     *  dialog: stays set after "Позже" so the sidebar can still offer it. */
    private val _available = MutableStateFlow<LatestRelease?>(null)
    val available: StateFlow<LatestRelease?> = _available.asStateFlow()

    /** Once the user taps "Позже", stop auto-popping the dialog for the session
     *  (the sidebar entry remains the way back in). */
    private var dialogDismissed = false

    init {
        check()
    }

    fun check() {
        viewModelScope.launch {
            val release = runCatching { UpdateRepository.checkForUpdate() }.getOrNull() ?: return@launch
            _available.value = release
            if (!dialogDismissed && _state.value is UpdateState.Idle) {
                _state.value = UpdateState.Available(release)
            }
        }
    }

    /** Begin (or re-begin, e.g. from the sidebar after a dismiss) the download. */
    fun startDownload() {
        val release = _available.value ?: _state.value.release ?: return
        viewModelScope.launch {
            _state.value = UpdateState.Downloading(0f, release)
            runCatching {
                UpdateRepository.download(getApplication<Application>().cacheDir, release) { p ->
                    _state.value = UpdateState.Downloading(p, release)
                }
            }.onSuccess {
                _state.value = UpdateState.Ready(it, release)
            }.onFailure {
                _state.value = UpdateState.Failed(it.message ?: "Не удалось загрузить", release)
            }
        }
    }

    fun install() {
        (_state.value as? UpdateState.Ready)?.let { UpdateRepository.install(getApplication(), it.file) }
    }

    /** "Позже": hide the dialog for the session. [available] stays set. */
    fun dismiss() {
        dialogDismissed = true
        _state.value = UpdateState.Idle
    }

    private val UpdateState.release: LatestRelease?
        get() = when (this) {
            is UpdateState.Available -> release
            is UpdateState.Downloading -> release
            is UpdateState.Ready -> release
            is UpdateState.Failed -> release
            UpdateState.Idle -> null
        }
}
