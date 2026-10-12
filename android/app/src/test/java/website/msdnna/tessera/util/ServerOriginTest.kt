package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import org.junit.Test

/**
 * The "is this our server?" question, which two WebViews answer with something
 * the session pays for: the `Authorization` header on inline images, and
 * permission to load inside the editor's WebView, where the `AndroidDoc` bridge
 * (with `requestToken`) is exposed.
 *
 * Both used to ask it as `url.startsWith(serverRoot)`. The cases below are the
 * ones a text prefix gets wrong — each begins with the root's text and each
 * resolves to a different host. [hostile] pins that second half down, so the
 * test fails if the claim "this really goes elsewhere" ever stops being true.
 */
class ServerOriginTest {
    private val root = "https://tessera.example"

    /** The host the request would actually reach, per the client's own parser. */
    private fun hostOf(url: String): String? = url.toHttpUrlOrNull()?.host

    private fun hostile(url: String) {
        assertThat(url.startsWith(root)).isTrue() // a string prefix says "ours"
        assertThat(hostOf(url)).isNotEqualTo("tessera.example") // the bytes go elsewhere
        assertThat(isOwnServerUrl(url, root)).isFalse()
    }

    @Test
    fun `a host that merely starts with ours is not ours`() {
        hostile("https://tessera.example.attacker.test/steal")
    }

    @Test
    fun `the root in the userinfo is not ours`() {
        hostile("https://tessera.example@attacker.test/steal")
        hostile("https://tessera.example:pass@attacker.test/steal")
    }

    @Test
    fun `a longer port that starts with ours is not ours`() {
        assertThat(isOwnServerUrl("http://tessera.example:8090/x", "http://tessera.example:80")).isFalse()
    }

    @Test
    fun `our own resources are still ours`() {
        assertThat(isOwnServerUrl("https://tessera.example/api/uploads/a.png", root)).isTrue()
        assertThat(isOwnServerUrl("https://tessera.example", root)).isTrue()
        // Trailing slash on the root, and the port written out explicitly.
        assertThat(isOwnServerUrl("https://tessera.example/api/uploads/a.png", "https://tessera.example/")).isTrue()
        assertThat(isOwnServerUrl("https://tessera.example:443/api/uploads/a.png", root)).isTrue()
        assertThat(isOwnServerUrl("HTTPS://TESSERA.EXAMPLE/api/uploads/a.png", root)).isTrue()
        assertThat(isOwnServerUrl("http://127.0.0.1:8090/api/uploads/a.png", "http://127.0.0.1:8090/api")).isTrue()
    }

    @Test
    fun `a different scheme or port is a different server`() {
        assertThat(isOwnServerUrl("http://tessera.example/api/uploads/a.png", root)).isFalse()
        assertThat(isOwnServerUrl("https://tessera.example:8443/api/uploads/a.png", root)).isFalse()
    }

    @Test
    fun `a root under a path owns only its own subtree`() {
        val subpath = "https://host.example/tessera"
        assertThat(isOwnServerUrl("https://host.example/tessera/api/uploads/a.png", subpath)).isTrue()
        assertThat(isOwnServerUrl("https://host.example/tessera", subpath)).isTrue()
        assertThat(isOwnServerUrl("https://host.example/tessera-other/x", subpath)).isFalse()
        assertThat(isOwnServerUrl("https://host.example/other", subpath)).isFalse()
    }

    @Test
    fun `what we cannot read is not ours`() {
        // No session yet: nothing is our server, so nothing gets the header.
        assertThat(isOwnServerUrl("https://tessera.example/api/uploads/a.png", "")).isFalse()
        assertThat(isOwnServerUrl("", root)).isFalse()
        // Schemes the HTTP client does not speak, including the base the reader
        // renders from and the one that would run script in the page.
        assertThat(isOwnServerUrl("file:///android_asset/richcontent/index.html", root)).isFalse()
        assertThat(isOwnServerUrl("javascript:alert(1)", root)).isFalse()
        assertThat(isOwnServerUrl("not a url", root)).isFalse()
    }
}
