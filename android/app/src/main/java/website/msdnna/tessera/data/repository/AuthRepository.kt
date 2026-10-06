package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.model.AuthProviders
import website.msdnna.tessera.data.model.AuthResponse
import website.msdnna.tessera.data.model.LoginRequest
import website.msdnna.tessera.data.model.Preferences
import website.msdnna.tessera.data.model.RefreshRequest
import website.msdnna.tessera.data.model.RegisterRequest
import website.msdnna.tessera.data.model.User
import website.msdnna.tessera.reminders.ReminderScheduler

/**
 * Auth flows backed by the network. On success it pushes the new token pair
 * into [RetrofitClient] (so subsequent calls are authenticated) and persists
 * the session via [AppPreferences].
 */
class AuthRepository {
    private val prefs get() = AppContainer.prefs

    suspend fun login(email: String, password: String): User {
        val res = AppContainer.api().login(LoginRequest(email.trim(), password))
        return persist(res)
    }

    suspend fun register(email: String, name: String, password: String): User {
        val res = AppContainer.api().register(RegisterRequest(email.trim(), name.trim(), password))
        return persist(res)
    }

    /** Which external login providers the server offers (drives the OAuth button). */
    suspend fun providers(): AuthProviders = AppContainer.api().authProviders()

    /** Completes an OAuth login: the deep link already carried the token pair, so
     *  push it into the client, fetch the profile, then persist the session
     *  atomically. Mirrors the web `OAuthCallbackView`. */
    suspend fun loginWithTokens(access: String, refresh: String): User {
        RetrofitClient.authToken = access
        RetrofitClient.refreshToken = refresh
        val me = AppContainer.api().me()
        val user = me.user ?: User()
        prefs.setSession(access to refresh, user)
        prefs.setPreferences(me.preferences ?: Preferences())
        return user
    }

    /** Validates the stored token against /auth/me, refreshing the profile + prefs. */
    suspend fun verify(): User {
        val me = AppContainer.api().me()
        val user = me.user ?: User()
        prefs.setUser(user)
        prefs.setPreferences(me.preferences ?: Preferences())
        return user
    }

    /**
     * Drops every local trace of the session. Kept separate from [logout] because
     * the expiry paths reach here with a token the server has already refused —
     * a revoke call would say nothing and only delay the login screen behind a
     * round-trip (mirrors the web `clearSession`).
     *
     * «Local trace» здесь включает будильники напоминаний (#2932). На вебе
     * стирать нечего — там напоминания только хранятся; на телефоне локальный
     * будильник и есть вся доставка, а его текст лежит в `PendingIntent`, вне
     * DataStore и вне токена. Поэтому до этой уборки личное напоминание всплывало
     * в свой час уже после выхода — и на общем устройстве доставалось тому, кто
     * вошёл следующим. Планировщику нужен контекст приложения; на путях, где
     * контейнер ещё не поднят, уборка молча пропускается — выход важнее неё.
     */
    suspend fun clearSession() {
        RetrofitClient.authToken = ""
        RetrofitClient.refreshToken = ""
        runCatching { ReminderScheduler.cancelAllScheduled(AppContainer.appContext) }
        prefs.clearSession()
    }

    /**
     * Signs out on purpose: the refresh token is revoked on the server first,
     * then the local session goes. Without the revoke the row stays tradable for
     * its full 30-day TTL — rotation renews it — so "Выйти" ended the session on
     * this screen only, and a copy of the token taken off the device outlived it
     * by a month. Best-effort: offline must not leave anyone stuck signed in, and
     * the endpoint is idempotent, so a lost response costs nothing.
     */
    suspend fun logout() {
        val refresh = RetrofitClient.refreshToken
        if (refresh.isNotBlank()) {
            runCatching { AppContainer.api().logout(RefreshRequest(refresh)) }
        }
        clearSession()
    }

    /** Adopts a freshly issued session. Not private because a password change
     *  also mints one (see [ProfileRepository.changePassword]) and both paths
     *  have to store it the same way. */
    internal suspend fun persist(res: AuthResponse): User {
        RetrofitClient.authToken = res.accessToken
        RetrofitClient.refreshToken = res.refreshToken
        val user = res.user ?: User()
        prefs.setSession(res.accessToken to res.refreshToken, user)
        prefs.setPreferences(res.preferences ?: Preferences())
        return user
    }
}
