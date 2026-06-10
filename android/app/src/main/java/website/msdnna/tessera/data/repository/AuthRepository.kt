package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.model.AuthResponse
import website.msdnna.tessera.data.model.LoginRequest
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

    /** Validates the stored token against /auth/me, refreshing the profile. */
    suspend fun verify(): User {
        val me = AppContainer.api().me()
        prefs.setUser(me)
        return me
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
        return user
    }
}
