package website.msdnna.tessera.data.repository

import kotlinx.coroutines.flow.first
import okhttp3.MediaType.Companion.toMediaTypeOrNull
import okhttp3.MultipartBody
import okhttp3.RequestBody.Companion.toRequestBody
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.PasswordChange
import website.msdnna.tessera.data.model.Preferences
import website.msdnna.tessera.data.model.ProfileUpdate
import website.msdnna.tessera.data.model.User

/**
 * Self-service profile / preferences / avatar (U1). Each call persists the
 * server's response back into [AppPreferences] so the cached profile + theme
 * stay in sync without a re-fetch.
 */
class ProfileRepository {
    private val prefs get() = AppContainer.prefs
    private val api get() = AppContainer.api()

    suspend fun updateProfile(p: ProfileUpdate): User {
        val u = api.updateProfile(p)
        prefs.setUser(u)
        return u
    }

    suspend fun changePassword(current: String, next: String) =
        api.changePassword(PasswordChange(current, next))

    /** Cache preferences optimistically (instant theme change), then sync to the
     *  server; re-cache the server-normalised result if the PUT succeeds. */
    suspend fun savePreferences(p: Preferences): Preferences {
        prefs.setPreferences(p)
        return runCatching { api.updatePreferences(p) }.getOrNull()?.also { prefs.setPreferences(it) } ?: p
    }

    suspend fun uploadAvatar(bytes: ByteArray, filename: String, mime: String?): String {
        val media = (mime ?: "image/*").toMediaTypeOrNull()
        val part = MultipartBody.Part.createFormData("avatar", filename, bytes.toRequestBody(media))
        val res = api.uploadAvatar(part)
        prefs.user.first()?.let { prefs.setUser(it.copy(avatarUrl = res.avatarUrl)) }
        return res.avatarUrl
    }

    suspend fun deleteAvatar() {
        api.deleteAvatar()
        prefs.user.first()?.let { prefs.setUser(it.copy(avatarUrl = "")) }
    }
}
