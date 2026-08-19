package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * One per-user "seen once" flag (`GET/POST users/me/acknowledgements`). The key is
 * opaque to the server — the clients own its shape (`whatsnew:<version>`,
 * `spotlight:<navKey>`, `getstarted:<step>`) and share it across web and Android.
 */
data class Acknowledgement(
    @SerializedName("key") val key: String = "",
    @SerializedName("ack_at") val ackAt: String = "",
)

/** Body of `POST users/me/acknowledgements` (idempotent — the first timestamp wins). */
data class AckRequest(@SerializedName("key") val key: String)

/** `GET version` — the API's own version, shown next to the app's. `commit` and
 *  `built_at` are absent in a dev build, so both default to empty. */
data class ApiVersion(
    @SerializedName("api") val api: String = "",
    @SerializedName("commit") val commit: String = "",
    @SerializedName("built_at") val builtAt: String = "",
)
