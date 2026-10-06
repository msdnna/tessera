package website.msdnna.tessera.util

import java.net.URLDecoder
import java.security.SecureRandom

/** Session handed back by the OAuth deep link, or the reason there is none. */
sealed interface OAuthHandoff {
    /** Tokens plus the nonce the backend echoed — worthless until [handoffAccepted] says so. */
    data class Success(val access: String, val refresh: String, val state: String) : OAuthHandoff

    data class Error(val code: String) : OAuthHandoff
}

/** Bound on the nonce we mint and accept; matches the backend's `oauthAppStateMax`. */
private const val OAUTH_NONCE_BYTES = 16

/**
 * newOAuthNonce mints the value that ties a handoff to a login *this app* started
 * (#2932). `tessera://` is an OS-wide scheme: any web page open in the phone's browser
 * can navigate to `tessera://oauth/callback#access_token=…`, and without this check the
 * app swallows it and signs the user into whatever account the attacker minted — over a
 * live session, silently (session fixation). The desktop client has carried this since
 * #2932; Android went out with the same deep link and no nonce at all.
 *
 * Hex, so it survives the backend's alphanumeric filter on `app_state` — anything it
 * rejects comes back empty and every real login would be refused.
 */
fun newOAuthNonce(): String {
    val buf = ByteArray(OAUTH_NONCE_BYTES)
    SecureRandom().nextBytes(buf)
    return buf.joinToString("") { "%02x".format(it) }
}

/**
 * handoffAccepted decides whether a deep link belongs to a login we started. Nothing
 * pending means nobody pressed the button — that handoff is unsolicited by definition.
 * A blank echo means the backend never saw our nonce (an older server, or a forged link
 * that simply omitted it), which is exactly the case that must not pass.
 */
fun handoffAccepted(pending: String?, received: String?): Boolean =
    !pending.isNullOrBlank() && !received.isNullOrBlank() && pending == received

/**
 * parseOAuthFragment pulls the handoff out of the deep link's URL fragment (the payload
 * rides there so it never reaches the server logs). Takes the raw, still-encoded
 * fragment; returns null only for a link that carries nothing at all.
 */
fun parseOAuthFragment(fragment: String?): OAuthHandoff {
    val frag = fragment ?: return OAuthHandoff.Error("no_data")
    val params = frag.split("&").mapNotNull { part ->
        val i = part.indexOf('=')
        if (i <= 0) {
            null
        } else {
            runCatching {
                URLDecoder.decode(part.substring(0, i), "UTF-8") to
                    URLDecoder.decode(part.substring(i + 1), "UTF-8")
            }.getOrNull()
        }
    }.toMap()
    params["oauth_error"]?.let { return OAuthHandoff.Error(it) }
    val access = params["access_token"].orEmpty()
    val refresh = params["refresh_token"].orEmpty()
    if (access.isBlank() || refresh.isBlank()) return OAuthHandoff.Error("no_tokens")
    return OAuthHandoff.Success(access, refresh, params["state"].orEmpty())
}
