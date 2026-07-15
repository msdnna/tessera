package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.model.AuthProviders
import website.msdnna.tessera.data.model.AuthResponse
import website.msdnna.tessera.data.model.LoginRequest
import website.msdnna.tessera.data.model.Preferences
import website.msdnna.tessera.data.model.RegisterRequest
import website.msdnna.tessera.data.model.User

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

    suspend fun logout() {
        RetrofitClient.authToken = ""
        RetrofitClient.refreshToken = ""
        prefs.clearSession()
    }

    private suspend fun persist(res: AuthResponse): User {
        RetrofitClient.authToken = res.accessToken
        RetrofitClient.refreshToken = res.refreshToken
        val user = res.user ?: User()
        prefs.setSession(res.accessToken to res.refreshToken, user)
        prefs.setPreferences(res.preferences ?: Preferences())
        return user
    }
}
