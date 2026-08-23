package website.msdnna.tessera.ui

import android.app.Activity
import androidx.annotation.StringRes
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.view.WindowCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.android.awaitFrame
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import website.msdnna.tessera.R
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.model.Preferences
import website.msdnna.tessera.data.repository.AuthRepository
import website.msdnna.tessera.data.repository.ProfileRepository
import website.msdnna.tessera.ui.components.LoadingCaptions
import website.msdnna.tessera.ui.components.MtLogo
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.screens.AuthScreen
import website.msdnna.tessera.ui.screens.MainScreen
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraTheme
import website.msdnna.tessera.ui.theme.accentByKey
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.isAuthError

/** Upper bound on the startup session check before the splash gives up. */
private const val VERIFY_TIMEOUT_MS = 30_000L

private val BrandPurple = Color(0xFF7C6CFF)

/** Result of the GitLab OAuth deep link handed up from [MainActivity]. */
sealed interface OAuthResult {
    data class Success(val access: String, val refresh: String) : OAuthResult
    data class Error(val message: String) : OAuthResult
}

/** Maps a backend OAuth error code to a message resource (mirrors web LoginView).
 *  The code travels instead of the text, so the string is resolved inside the
 *  localized tree — see [AppLocale]. */
@StringRes
fun oauthErrorRes(code: String): Int = when (code) {
    "not_configured" -> R.string.auth_oauth_not_configured
    "state_mismatch" -> R.string.auth_oauth_state_mismatch
    "exchange_failed", "userinfo_failed" -> R.string.auth_oauth_exchange_failed
    "account_disabled" -> R.string.auth_oauth_account_disabled
    "no_tokens", "no_data" -> R.string.auth_oauth_incomplete
    else -> R.string.auth_oauth_failed
}

/** Outcome of the startup session check, driving the gate below. */
private sealed interface Boot {
    data object Loading : Boot
    data object Done : Boot // proceed to auth or main per the token
    data object ConnectError : Boot // the API server is unreachable
    data object AuthError : Boot // session invalid / account deactivated
}

/**
 * Root of the Compose tree. Owns theme prefs, the session gate (splash →
 * auth → main) and the RetrofitClient ↔ DataStore token wiring.
 */
@Composable
fun AppRoot(
    openTaskId: String? = null,
    onOpenTaskHandled: () -> Unit = {},
    oauthResult: OAuthResult? = null,
    onOAuthHandled: () -> Unit = {},
) {
    val prefs = AppContainer.prefs
    val scope = rememberCoroutineScope()
    val authRepo = remember { AuthRepository() }
    val profileRepo = remember { ProfileRepository() }
    // The backend's OAuth error *code*, not its text — resolved on the auth
    // screen, already inside the localized tree.
    var oauthError by remember { mutableStateOf<String?>(null) }

    val accentKey by prefs.accentKey.collectAsStateWithLifecycle(initialValue = "purple")
    val isDark by prefs.darkMode.collectAsStateWithLifecycle(initialValue = false)
    val tagPrefixMode by prefs.tagPrefixMode.collectAsStateWithLifecycle(initialValue = "name")
    val preferences by prefs.preferences.collectAsStateWithLifecycle(initialValue = Preferences())
    val token by prefs.authToken.collectAsStateWithLifecycle(initialValue = "")
    val user by prefs.user.collectAsStateWithLifecycle(initialValue = null)
    val serverUrl by prefs.serverUrl.collectAsStateWithLifecycle(initialValue = AppContainer.serverUrl)

    var boot by remember { mutableStateOf<Boot>(Boot.Loading) }
    var bootNonce by remember { mutableIntStateOf(0) }

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

    // Bootstrap (re-runnable via `bootNonce` for the error screens' «Retry»):
    // prime RetrofitClient + AppContainer from persisted state, then hold the
    // loading splash while we confirm the server is reachable and the session is
    // valid (bounded by a 30s timeout). The outcome drives the gate below:
    //   • no token / valid session  → Done   (auth screen / board)
    //   • unreachable or timed out   → ConnectError
    //   • 401/403 (expired/blocked)  → AuthError
    LaunchedEffect(bootNonce) {
        boot = Boot.Loading
        val url = prefs.serverUrl.first()
        val access = prefs.authToken.first()
        val refresh = prefs.refreshToken.first()
        AppContainer.serverUrl = url
        RetrofitClient.authToken = access
        RetrofitClient.refreshToken = refresh
        if (access.isBlank()) {
            boot = Boot.Done
            return@LaunchedEffect
        }
        boot = runCatching { withTimeoutOrNull(VERIFY_TIMEOUT_MS) { authRepo.verify() } }
            .fold(
                onSuccess = { if (it == null) Boot.ConnectError else Boot.Done },
                onFailure = { if (isAuthError(it)) Boot.AuthError else Boot.ConnectError },
            )
    }

    // A returning OAuth deep link: persist the handed-back session (→ token flow
    // flips the gate to Main), or surface the error on the auth screen.
    LaunchedEffect(oauthResult) {
        when (val r = oauthResult) {
            null -> {}

            is OAuthResult.Success -> {
                oauthError = null
                runCatching { authRepo.loginWithTokens(r.access, r.refresh) }
                    .onFailure { boot = if (isAuthError(it)) Boot.AuthError else Boot.ConnectError }
                onOAuthHandled()
            }

            is OAuthResult.Error -> {
                oauthError = r.message
                onOAuthHandled()
            }
        }
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

    fun exitApp() {
        (view.context as? Activity)?.finish()
    }

    // Language comes from the profile, not the device — the whole tree below
    // resolves its strings in it (#2803).
    AppLocale(language = preferences.language) {
        TesseraTheme(accent = accentByKey(accentKey), isDark = isDark, tagPrefixMode = tagPrefixMode) {
            Surface(Modifier.fillMaxSize(), color = Tessera.colors.bg) {
                when {
                    boot is Boot.Loading -> BootLoading()

                    boot is Boot.ConnectError -> BootError(
                        title = stringResource(R.string.gate_offline_title),
                        message = stringResource(R.string.gate_offline_message),
                        primaryLabel = stringResource(R.string.gate_offline_retry),
                        onPrimary = { bootNonce++ },
                        onExit = ::exitApp,
                    )

                    boot is Boot.AuthError -> BootError(
                        title = stringResource(R.string.gate_session_title),
                        message = stringResource(R.string.gate_session_message),
                        primaryLabel = stringResource(R.string.gate_session_relogin),
                        onPrimary = {
                            scope.launch { authRepo.logout() }
                            boot = Boot.Done
                        },
                        onExit = ::exitApp,
                    )

                    token.isBlank() -> AuthScreen(
                        serverUrl = serverUrl,
                        onServerUrlChange = { scope.launch { prefs.setServerUrl(it) } },
                        isDark = isDark,
                        // Pre-login the theme lives only in local prefs (no user yet);
                        // it's reconciled with the server pref after sign-in.
                        onToggleTheme = { scope.launch { prefs.setDarkMode(!isDark) } },
                        oauthErrorCode = oauthError,
                        onOAuthErrorShown = { oauthError = null },
                    )

                    else -> MainScreen(
                        user = user,
                        isDark = isDark,
                        accentKey = accentKey,
                        openTaskId = openTaskId,
                        onOpenTaskHandled = onOpenTaskHandled,
                        onAccentChange = { scope.launch { profileRepo.savePreferences(preferences.copy(accent = it)) } },
                        onToggleDark = {
                            scope.launch {
                                profileRepo.savePreferences(preferences.copy(theme = if (isDark) "light" else "dark"))
                            }
                        },
                        onLogout = { scope.launch { authRepo.logout() } },
                    )
                }
            }
        }
    }
}

/** Full-bleed purple gradient — the shared backdrop for the launch splash and
 *  its error screens. Fixed brand purple (not the user's accent), since the
 *  accent pref may still be loading; reproduces the bundle's
 *  #6D5FE0→#7C6CFF→#9183FF diagonal almost exactly. */
@Composable
private fun PurpleBackdrop(content: @Composable () -> Unit) {
    Box(
        Modifier.fillMaxSize().background(accentGradient(BrandPurple)),
        contentAlignment = Alignment.Center,
    ) { content() }
}

@Composable
private fun BootLoading() {
    // Brief frame yield keeps the splash from flashing when prefs load instantly.
    LaunchedEffect(Unit) { awaitFrame() }
    PurpleBackdrop {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            TesseraLoader(size = 108.dp, color = Color.White)
            Spacer(Modifier.height(24.dp))
            // Reassuring captions fade in only if the connect/verify drags past 5s.
            LoadingCaptions(color = Color.White.copy(alpha = 0.82f))
        }
    }
}

/** A startup error on the purple backdrop: brand mark, a message, a white CTA,
 *  and a ghost exit link. Text/buttons are light to read on purple (login style). */
@Composable
private fun BootError(
    title: String,
    message: String,
    primaryLabel: String,
    onPrimary: () -> Unit,
    onExit: () -> Unit,
) {
    PurpleBackdrop {
        Column(
            Modifier.fillMaxWidth().padding(horizontal = 36.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            MtLogo(size = 56.dp, tint = Color.White, gradient = false)
            Spacer(Modifier.height(6.dp))
            Text(title, color = Color.White, fontSize = 20.sp, fontWeight = FontWeight.SemiBold)
            Text(
                message,
                color = Color.White.copy(alpha = 0.78f),
                fontSize = 14.sp,
                textAlign = TextAlign.Center,
                modifier = Modifier.widthIn(max = 320.dp),
            )
            Spacer(Modifier.height(6.dp))
            BootPrimaryButton(primaryLabel, onPrimary)
            Box(
                Modifier
                    .clickableNoRipple(onClick = onExit)
                    .padding(horizontal = 18.dp, vertical = 10.dp),
            ) {
                Text(
                    stringResource(R.string.gate_exit),
                    color = Color.White.copy(alpha = 0.85f),
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Medium,
                )
            }
        }
    }
}

/** The high-contrast white CTA (purple label) — primary action on purple. */
@Composable
private fun BootPrimaryButton(text: String, onClick: () -> Unit) {
    Box(
        Modifier
            .widthIn(min = 220.dp)
            .background(Color.White, RoundedCornerShape(RadiusMd))
            .clickableNoRipple(onClick = onClick)
            .heightIn(min = 48.dp)
            .padding(horizontal = 18.dp, vertical = 12.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(text, color = BrandPurple, fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
    }
}
