package website.msdnna.tessera.util

import android.net.Uri
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

/**
 * The app's half of the editor bridge (#2894 §4).
 *
 * Everything here is invisible when it breaks, which is why it is tested at all:
 * a parameter under the wrong name gives a blank editor with no error anywhere,
 * and a status word the app fails to recognise would quietly read as
 * "сохранено" over text that never reached the server.
 *
 * Robolectric because [docEmbedUrl] percent-encodes through [Uri] and the
 * signals arrive as `org.json` — both are stubs in a plain JVM test.
 */
@RunWith(RobolectricTestRunner::class)
class DocEmbedTest {
    private fun params(url: String): Map<String, String?> {
        val parsed = Uri.parse(url)
        return parsed.queryParameterNames.associateWith { parsed.getQueryParameter(it) }
    }

    @Test
    fun `url carries the session, the workspace and the look`() {
        val url = docEmbedUrl(
            serverRoot = "https://tessera.example",
            slug = "plan",
            workspaceId = "ws-1",
            token = "tok-1",
            dark = true,
            language = "ru",
        )
        assertThat(url).startsWith("https://tessera.example/embed/document/plan?")
        assertThat(params(url)).containsExactly(
            "ws", "ws-1",
            "token", "tok-1",
            "theme", "dark",
            "lang", "ru",
        )
    }

    @Test
    fun `a trailing slash on the server does not double up`() {
        val url = docEmbedUrl("https://tessera.example/", "plan", "ws", "t", false, "en")
        assertThat(url).startsWith("https://tessera.example/embed/document/plan?")
        assertThat(url).doesNotContain("//embed")
    }

    @Test
    fun `light is spelled out rather than left to a default`() {
        val url = docEmbedUrl("https://x", "s", "w", "t", dark = false, language = "en")
        assertThat(params(url)["theme"]).isEqualTo("light")
    }

    @Test
    fun `a slug with characters a URL cannot hold survives the round trip`() {
        // Slugs are generated from the title, and a title can be anything —
        // Cyrillic, a space, a question mark that would otherwise start the
        // query string early and silently drop every parameter after it.
        val url = docEmbedUrl("https://x", "план 1?", "w", "t/k", dark = false, language = "ru")
        assertThat(Uri.parse(url).lastPathSegment).isEqualTo("план 1?")
        assertThat(params(url)["token"]).isEqualTo("t/k")
    }

    @Test
    fun `every status the page can report is understood`() {
        val cases = mapOf(
            "saved" to DocSaveStatus.SAVED,
            "dirty" to DocSaveStatus.DIRTY,
            "saving" to DocSaveStatus.SAVING,
            "conflict" to DocSaveStatus.CONFLICT,
            "error" to DocSaveStatus.ERROR,
        )
        for ((word, expected) in cases) {
            val signal = parseDocEditorSignal("""{"status":"$word","error":""}""")
            assertThat(signal.status).isEqualTo(expected)
        }
    }

    @Test
    fun `an unreadable report is an error, never a claim that the text is saved`() {
        assertThat(parseDocEditorSignal("not json").status).isEqualTo(DocSaveStatus.ERROR)
        assertThat(parseDocEditorSignal("{}").status).isEqualTo(DocSaveStatus.ERROR)
        assertThat(parseDocEditorSignal("""{"status":"whatever"}""").status)
            .isEqualTo(DocSaveStatus.ERROR)
    }

    @Test
    fun `the failure reason travels with an error`() {
        val signal = parseDocEditorSignal("""{"status":"error","error":"Сеть недоступна"}""")
        assertThat(signal.status).isEqualTo(DocSaveStatus.ERROR)
        assertThat(signal.message).isEqualTo("Сеть недоступна")
    }

    @Test
    fun `a tap on a block brings the anchor, the quote and the document's blocks`() {
        val target = parseDocAnnotate(
            """{"block_id":"b2","quote":"Второй абзац","blocks":["b1","b2","b3"]}""",
        )
        assertThat(target.blockId).isEqualTo("b2")
        assertThat(target.quote).isEqualTo("Второй абзац")
        assertThat(target.blockIds).containsExactly("b1", "b2", "b3").inOrder()
    }

    @Test
    fun `an unreadable tap opens the whole document rather than a wrong block`() {
        // Anchoring a new remark to a block the page never named would attach it
        // to whatever the app happened to remember — an empty target opens the
        // sheet on the document, which is merely less specific.
        assertThat(parseDocAnnotate("not json").blockId).isEmpty()
        assertThat(parseDocAnnotate("{}").blockIds).isEmpty()
        assertThat(parseDocAnnotate("""{"block_id":"b1"}""").quote).isEmpty()
    }

    @Test
    fun `the ready report names the document`() {
        val ready = parseDocEditorReady("""{"id":"doc-1","title":"План"}""")
        assertThat(ready.id).isEqualTo("doc-1")
        assertThat(ready.title).isEqualTo("План")
        // A document opened before its title arrives is not a crash.
        assertThat(parseDocEditorReady("nonsense").id).isEmpty()
    }
}
