package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/** Authenticated user profile — mirrors the backend `userDTO`. */
data class User(
    @SerializedName("id") val id: String = "",
    @SerializedName("email") val email: String = "",
    @SerializedName("name") val name: String = "",
    @SerializedName("is_admin") val isAdmin: Boolean = false,
    @SerializedName("provider") val provider: String = "local",
    @SerializedName("last_name") val lastName: String = "",
    @SerializedName("first_name") val firstName: String = "",
    @SerializedName("middle_name") val middleName: String = "",
    @SerializedName("bio") val bio: String = "",
    @SerializedName("company") val company: String = "",
    @SerializedName("job_title") val jobTitle: String = "",
    @SerializedName("avatar_url") val avatarUrl: String = "",
)

/** Per-user localizing + personalizing preferences (backend `user_preferences`). */
data class Preferences(
    @SerializedName("language") val language: String = "ru",
    @SerializedName("timezone") val timezone: String = "",
    @SerializedName("country") val country: String = "",
    @SerializedName("time_format") val timeFormat: String = "24h",
    @SerializedName("date_format") val dateFormat: String = "dd.MM.yyyy",
    @SerializedName("week_start") val weekStart: Int = 1,
    @SerializedName("theme") val theme: String = "system",
    @SerializedName("accent") val accent: String = "purple",
    @SerializedName("board_background") val boardBackground: String = "",
)

/** Response shape of /auth/login, /auth/register and /auth/refresh. */
data class AuthResponse(
    @SerializedName("access_token") val accessToken: String = "",
    @SerializedName("refresh_token") val refreshToken: String = "",
    @SerializedName("user") val user: User? = null,
    @SerializedName("preferences") val preferences: Preferences? = null,
)

/** Response shape of GET /auth/me (user + preferences). */
data class MeResponse(
    @SerializedName("user") val user: User? = null,
    @SerializedName("preferences") val preferences: Preferences? = null,
)

/** PATCH /users/me — profile fields (email/role not editable here). */
data class ProfileUpdate(
    @SerializedName("name") val name: String,
    @SerializedName("last_name") val lastName: String,
    @SerializedName("first_name") val firstName: String,
    @SerializedName("middle_name") val middleName: String,
    @SerializedName("bio") val bio: String,
    @SerializedName("company") val company: String,
    @SerializedName("job_title") val jobTitle: String,
)

/** PUT /users/me/password. */
data class PasswordChange(
    @SerializedName("current_password") val currentPassword: String,
    @SerializedName("new_password") val newPassword: String,
)

/** PATCH /workspaces/:id/members/:userId. */
data class RoleUpdate(
    @SerializedName("role") val role: String,
)

/** Response of PUT /users/me/avatar. */
data class AvatarResponse(
    @SerializedName("avatar_url") val avatarUrl: String = "",
)

data class LoginRequest(
    @SerializedName("email") val email: String,
    @SerializedName("password") val password: String,
)

data class RegisterRequest(
    @SerializedName("email") val email: String,
    @SerializedName("name") val name: String,
    @SerializedName("password") val password: String,
)

data class RefreshRequest(
    @SerializedName("refresh_token") val refreshToken: String,
)

/** Error envelope: the backend returns `{ "error": "..." }` on failures. */
data class ApiError(
    @SerializedName("error") val error: String = "",
)
