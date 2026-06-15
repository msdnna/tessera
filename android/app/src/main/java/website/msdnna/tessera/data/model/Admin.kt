package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/** An instance account as shown in the global admin panel (U3). */
data class AdminUser(
    @SerializedName("id") val id: String = "",
    @SerializedName("email") val email: String = "",
    @SerializedName("name") val name: String = "",
    @SerializedName("is_admin") val isAdmin: Boolean = false,
    @SerializedName("active") val active: Boolean = true,
    @SerializedName("email_verified") val emailVerified: Boolean = false,
    @SerializedName("created_at") val createdAt: String = "",
    @SerializedName("avatar_url") val avatarUrl: String = "",
)

data class SetActiveRequest(@SerializedName("active") val active: Boolean)
data class SetAdminRequest(@SerializedName("admin") val admin: Boolean)

/** A bare `{ "link": "..." }` response (admin-minted password-reset link). */
data class LinkResponse(@SerializedName("link") val link: String = "")
