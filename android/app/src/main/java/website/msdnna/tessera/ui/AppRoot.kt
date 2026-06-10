package website.msdnna.tessera.ui

import android.app.Activity
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.unit.dp
import androidx.core.view.WindowCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.android.awaitFrame
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.repository.AuthRepository
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.screens.AuthScreen
import website.msdnna.tessera.ui.screens.MainScreen
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraTheme
import website.msdnna.tessera.ui.theme.accentByKey
import website.msdnna.tessera.ui.theme.accentGradient

/**
 * Root of the Compose tree. Owns theme prefs, the session gate (splash →
 * auth → main) and the RetrofitClient ↔ DataStore token wiring.
 */
@Composable
fun AppRoot(
    openTaskId: String? = null,
    onOpenTaskHandled: () -> Unit = {},
) {
    val prefs = AppContainer.prefs
    val scope = rememberCoroutineScope()
    val authRepo = remember { AuthRepository() }

    val accentKey by prefs.accentKey.collectAsStateWithLifecycle(initialValue = "purple")
    val isDark by prefs.darkMode.collectAsStateWithLifecycle(initialValue = false)
    val token by prefs.authToken.collectAsStateWithLifecycle(initialValue = "")
    val user by prefs.user.collectAsStateWithLifecycle(initialValue = null)
    val serverUrl by prefs.serverUrl.collectAsStateWithLifecycle(initialValue = AppContainer.serverUrl)

    var ready by remember { mutableStateOf(false) }

    // Match the system status/navigation bar icon colours to the theme so they
    // stay legible (and don't glare) over the app background.
    val view = LocalView.current
    LaunchedEffect(isDark) {
        val window = (view.context as? Activity)?.window ?: return@LaunchedEffect
        WindowCompat.getInsetsController(window, view).apply {
            isAppearanceLightStatusBars = !isDark
            isAppearanceLightNavigationBars = !isDark
        }
    }

    // One-shot bootstrap: prime RetrofitClient + AppContainer from persisted
    // state before flipping `ready`, so the session gate sees a consistent
    // snapshot rather than the flow initial values.
    LaunchedEffect(Unit) {
        val url = prefs.serverUrl.first()
        val access = prefs.authToken.first()
        val refresh = prefs.refreshToken.first()
        AppContainer.serverUrl = url
        RetrofitClient.authToken = access
        RetrofitClient.refreshToken = refresh
        ready = true
        // Validate the stored session in the background (refreshes the profile).
        if (access.isNotBlank()) runCatching { authRepo.verify() }
    }

    // Keep the network client's server URL in sync with prefs changes.
    LaunchedEffect(serverUrl) {
        if (AppContainer.serverUrl != serverUrl) {
            AppContainer.serverUrl = serverUrl
            RetrofitClient.reset()
        }
    }

    // Bridge silent-refresh + hard-logout callbacks (fire on OkHttp threads).
    DisposableEffect(Unit) {
        RetrofitClient.onTokensRefreshed = { access, refresh ->
            scope.launch { prefs.setTokens(access, refresh) }
        }
        RetrofitClient.onUnauthorized = {
            scope.launch { authRepo.logout() }
        }
        onDispose {
            RetrofitClient.onTokensRefreshed = null
            RetrofitClient.onUnauthorized = null
        }
    }

    TesseraTheme(accent = accentByKey(accentKey), isDark = isDark) {
        Surface(Modifier.fillMaxSize(), color = Tessera.colors.bg) {
            when {
                !ready -> Splash()

                token.isBlank() -> AuthScreen(
                    serverUrl = serverUrl,
                    onServerUrlChange = { scope.launch { prefs.setServerUrl(it) } },
                )

                else -> MainScreen(
                    user = user,
                    isDark = isDark,
                    accentKey = accentKey,
                    openTaskId = openTaskId,
                    onOpenTaskHandled = onOpenTaskHandled,
                    onAccentChange = { scope.launch { prefs.setAccentKey(it) } },
                    onToggleDark = { scope.launch { prefs.setDarkMode(!isDark) } },
                    onLogout = { scope.launch { authRepo.logout() } },
                )
            }
        }
    }
}

@Composable
private fun Splash() {
    // Brief frame yield keeps the splash from flashing when prefs load instantly.
    LaunchedEffect(Unit) { awaitFrame() }
    // Brand launch splash: a white tessera tile spinning on the purple gradient.
    // The fixed brand purple (not the user's accent) is used here because the
    // accent pref may still be loading at this point. accentGradient(#7C6CFF)
    // reproduces the bundle's #6D5FE0→#7C6CFF→#9183FF diagonal almost exactly.
    Box(
        Modifier
            .fillMaxSize()
            .background(accentGradient(Color(0xFF7C6CFF))),
        contentAlignment = Alignment.Center,
    ) {
        TesseraLoader(size = 72.dp, color = Color.White, gradient = false)
    }
}
