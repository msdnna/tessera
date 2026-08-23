package website.msdnna.tessera.ui.viewmodels

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.repository.AuthRepository
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.util.errorMessage

data class AuthUiState(
    val loading: Boolean = false,
    val error: UiText? = null,
    val gitlabEnabled: Boolean = false,
)

/** Drives the login / register submission. Session persistence happens in the
 *  repository; the host observes prefs to advance past the auth phase. */
class AuthViewModel(
    private val repo: AuthRepository = AuthRepository(),
) : ViewModel() {
    private val _state = MutableStateFlow(AuthUiState())
    val state: StateFlow<AuthUiState> = _state.asStateFlow()

    fun login(email: String, password: String) = submit {
        repo.login(email, password)
    }

    fun register(email: String, name: String, password: String) = submit {
        repo.register(email, name, password)
    }

    /** Probe the server for enabled OAuth providers so the login screen can show
     *  the «Войти через GitLab» button. Best-effort — silence on failure. */
    fun loadProviders() {
        viewModelScope.launch {
            runCatching { repo.providers() }.getOrNull()?.let { p ->
                _state.update { it.copy(gitlabEnabled = p.gitlab) }
            }
        }
    }

    /** Экран отдаёт id ресурса, а не текст: строку резолвит сама композиция. */
    fun setError(@StringRes message: Int) = _state.update { it.copy(error = UiText.Res(message)) }

    fun clearError() = _state.update { it.copy(error = null) }

    private fun submit(block: suspend () -> Unit) {
        if (_state.value.loading) return
        _state.update { it.copy(loading = true, error = null) }
        viewModelScope.launch {
            val result = runCatching { block() }
            _state.update {
                it.copy(loading = false, error = result.exceptionOrNull()?.let(::errorMessage))
            }
        }
    }
}
