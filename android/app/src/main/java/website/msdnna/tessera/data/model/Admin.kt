package website.msdnna.tessera.data.model

import com.google.gson.JsonElement
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

/** GET /admin/oauth/gitlab — admin OAuth provider config. The client secret and
 *  service token are never returned; `has_*` only report whether one is stored. */
data class OAuthConfig(
    @SerializedName("provider") val provider: String = "gitlab",
    @SerializedName("client_id") val clientId: String = "",
    @SerializedName("gl_base_url") val glBaseUrl: String = "",
    @SerializedName("enabled") val enabled: Boolean = false,
    @SerializedName("org_map") val orgMap: JsonElement? = null,
    @SerializedName("has_secret") val hasSecret: Boolean = false,
    @SerializedName("has_service_token") val hasServiceToken: Boolean = false,
)

/** PUT /admin/oauth/gitlab. Empty `client_secret`/`service_token` keep the stored
 *  values (so they needn't be re-entered on every edit). */
data class OAuthConfigRequest(
    @SerializedName("client_id") val clientId: String,
    @SerializedName("client_secret") val clientSecret: String,
    @SerializedName("gl_base_url") val glBaseUrl: String,
    @SerializedName("enabled") val enabled: Boolean,
    @SerializedName("org_map") val orgMap: JsonElement,
    @SerializedName("service_token") val serviceToken: String,
)
