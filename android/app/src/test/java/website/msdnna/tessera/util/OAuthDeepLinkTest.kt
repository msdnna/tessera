package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * The deep link's own arithmetic, away from the live-backend tier: what counts as our
 * handoff, and what the nonce has to look like for the backend to echo it at all.
 */
class OAuthDeepLinkTest {
    @Test
    fun `a handoff is ours only when it echoes the pending nonce`() {
        assertThat(handoffAccepted("abc123", "abc123")).isTrue()
        assertThat(handoffAccepted("abc123", "def456")).isFalse()
        // Nothing pending — nobody started a login, so this link came from elsewhere.
        assertThat(handoffAccepted(null, "abc123")).isFalse()
        assertThat(handoffAccepted("", "")).isFalse()
        // A link that simply omits the state must not read as "no objection".
        assertThat(handoffAccepted("abc123", "")).isFalse()
        assertThat(handoffAccepted("abc123", null)).isFalse()
    }

    @Test
    fun `the nonce survives the backend's alphanumeric filter`() {
        val nonce = newOAuthNonce()
        // `oauthAppState` (backend/handlers/oauth.go) drops anything else on the floor,
        // which would leave every real login without an echo — and refused.
        assertThat(nonce).matches("[0-9a-f]{32}")
        assertThat(nonce).isNotEqualTo(newOAuthNonce())
    }

    @Test
    fun `the fragment yields tokens with the state alongside them`() {
        val handoff = parseOAuthFragment("access_token=a.b.c&refresh_token=r-1&state=nonce9")
        assertThat(handoff).isEqualTo(OAuthHandoff.Success("a.b.c", "r-1", "nonce9"))
    }

    @Test
    fun `a forged fragment parses but carries no state to match`() {
        val handoff = parseOAuthFragment("access_token=stolen&refresh_token=stolen-r")
        assertThat(handoff).isEqualTo(OAuthHandoff.Success("stolen", "stolen-r", ""))
        assertThat(handoffAccepted("pending", (handoff as OAuthHandoff.Success).state)).isFalse()
    }

    @Test
    fun `errors and empty payloads keep their codes`() {
        assertThat(parseOAuthFragment("oauth_error=state_mismatch"))
            .isEqualTo(OAuthHandoff.Error("state_mismatch"))
        assertThat(parseOAuthFragment(null)).isEqualTo(OAuthHandoff.Error("no_data"))
        assertThat(parseOAuthFragment("state=nonce9")).isEqualTo(OAuthHandoff.Error("no_tokens"))
        // Percent-encoding is undone, so a token is usable as handed over.
        assertThat(parseOAuthFragment("access_token=a%2Bb&refresh_token=r%2Fc&state=n"))
            .isEqualTo(OAuthHandoff.Success("a+b", "r/c", "n"))
    }
}
