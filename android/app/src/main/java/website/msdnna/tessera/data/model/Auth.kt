package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/** Authenticated user profile — mirrors the backend `userDTO`. */
data class User(
    @SerializedName("id") val id: String = "",
    @SerializedName("email") val email: String = "",
    @SerializedName("name") val name: String = "",
    @SerializedName("is_admin") val isAdmin: Boolean = false,
)

/** Response shape of /auth/login, /auth/register and /auth/refresh. */
data class AuthResponse(
    @SerializedName("access_token") val accessToken: String = "",
    @SerializedName("refresh_token") val refreshToken: String = "",
    @SerializedName("user") val user: User? = null,
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
