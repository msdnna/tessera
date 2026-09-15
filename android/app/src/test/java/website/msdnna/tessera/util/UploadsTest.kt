package website.msdnna.tessera.util

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

/**
 * The two task upload paths, refused before the bytes travel (#2932).
 *
 * The web half of this rule lives in `frontend/src/utils/upload.js` and says why
 * it exists: a 40 MB file otherwise goes over the wire in full, and the refusal
 * that comes back is the handler's hardcoded *Russian* sentence, shown verbatim
 * to a reader who chose English. On a phone it is worse in two more ways — the
 * picker reads the whole file into a ByteArray before sending it, and the inline
 * image path swallows the failure entirely, so an over-cap image simply never
 * appears with nothing said.
 *
 * The boundaries below are the server's own comparisons (`fileHeader.Size >
 * maxAttachmentBytes`, `> maxMediaBytes`), so *at* the cap must pass on both
 * sides: a client that refuses a file the server would have taken is a feature
 * quietly removed.
 */
@RunWith(RobolectricTestRunner::class)
class UploadsTest {
    private val appContext: Context get() = ApplicationProvider.getApplicationContext()

    private fun res(language: String) = appContext.withLanguage(language).resources

    private fun size(language: String, bytes: Long): String = attachmentSizeText(res(language), bytes)

    @Test
    fun `an attachment is refused only above the server's cap`() {
        assertThat(attachmentPickError(1L)).isEqualTo(PickRefusal.NONE)
        assertThat(attachmentPickError(ATTACHMENT_MAX_BYTES - 1)).isEqualTo(PickRefusal.NONE)
        assertThat(attachmentPickError(ATTACHMENT_MAX_BYTES)).isEqualTo(PickRefusal.NONE)
        assertThat(attachmentPickError(ATTACHMENT_MAX_BYTES + 1)).isEqualTo(PickRefusal.TOO_BIG)
        assertThat(attachmentPickError(40L * 1024 * 1024)).isEqualTo(PickRefusal.TOO_BIG)
    }

    @Test
    fun `the caps are the ones the server compares against`() {
        assertThat(ATTACHMENT_MAX_BYTES).isEqualTo(25L * 1024 * 1024)
        assertThat(MEDIA_MAX_BYTES).isEqualTo(8L * 1024 * 1024)
        // `mediaExts` in backend/handlers/attachments.go, in its order.
        assertThat(MEDIA_TYPES).containsExactly(
            "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp",
        ).inOrder()
    }

    @Test
    fun `an unreadable size is left to the server`() {
        // The picker cannot always say how big a file is. Refusing on that would
        // make the phone stricter than the server for no reason the user can act
        // on — the upload goes ahead and the server decides.
        assertThat(attachmentPickError(null)).isEqualTo(PickRefusal.NONE)
        assertThat(attachmentPickError(-1L)).isEqualTo(PickRefusal.NONE)
        assertThat(mediaPickError(null, "image/png")).isEqualTo(PickRefusal.NONE)
    }

    @Test
    fun `an inline image is refused above its own, smaller cap`() {
        assertThat(mediaPickError(MEDIA_MAX_BYTES, "image/png")).isEqualTo(PickRefusal.NONE)
        assertThat(mediaPickError(MEDIA_MAX_BYTES + 1, "image/png")).isEqualTo(PickRefusal.TOO_BIG)
        // Well under the attachment cap, well over the media one: the two paths
        // are not the same rule, and a 12 MB photo is the ordinary case.
        assertThat(mediaPickError(12L * 1024 * 1024, "image/jpeg")).isEqualTo(PickRefusal.TOO_BIG)
        assertThat(attachmentPickError(12L * 1024 * 1024)).isEqualTo(PickRefusal.NONE)
    }

    @Test
    fun `size is answered before type`() {
        // A 12 MB SVG reads as "too large" on both sides, rather than inviting a
        // smaller SVG that would be refused anyway.
        assertThat(mediaPickError(12L * 1024 * 1024, "image/svg+xml")).isEqualTo(PickRefusal.TOO_BIG)
    }

    @Test
    fun `a type the server has no allowlist entry for is refused`() {
        // SVG matches the picker's `image/*` filter and is absent from mediaExts
        // on purpose — it is a script-bearing document served on the app's origin.
        assertThat(mediaPickError(1024L, "image/svg+xml")).isEqualTo(PickRefusal.BAD_TYPE)
        assertThat(mediaPickError(1024L, "image/heic")).isEqualTo(PickRefusal.BAD_TYPE)
        assertThat(mediaPickError(1024L, "application/pdf")).isEqualTo(PickRefusal.BAD_TYPE)
    }

    @Test
    fun `the type check is one-sided`() {
        // The server sniffs the bytes, so a resolver that reports nothing (or
        // reports a case the allowlist spells differently) must not be refused here.
        assertThat(mediaPickError(1024L, null)).isEqualTo(PickRefusal.NONE)
        assertThat(mediaPickError(1024L, "")).isEqualTo(PickRefusal.NONE)
        assertThat(mediaPickError(1024L, "IMAGE/PNG")).isEqualTo(PickRefusal.NONE)
        assertThat(mediaPickError(1024L, "image/jpeg; charset=binary")).isEqualTo(PickRefusal.NONE)
    }

    // ── the size of an attachment already uploaded ──────────────────────────
    //
    // The row under the file name. It used to format the number itself with
    // `Locale.US`, which is half the rule: the unit was taken from the catalogue
    // precisely so it would follow the profile, and then the separator was
    // nailed to a point. The same file was weighed «1.4 МБ» here and «1,4 МБ» in
    // a document and in a call's chat — inside one Russian interface.

    @Test
    fun `the separator is written in the language the row is read in`() {
        assertThat(size("ru", (1.4 * (1L shl 20)).toLong())).isEqualTo("1,4 МБ")
        assertThat(size("en", (1.4 * (1L shl 20)).toLong())).isEqualTo("1.4 MB")
    }

    @Test
    fun `anything scaled keeps one fraction digit`() {
        // `%.0f` read 14.6 KB as a flat «15 КБ» — a number that disagreed with
        // every other place the product weighs the same file.
        assertThat(size("ru", 15_000L)).isEqualTo("14,6 КБ")
        assertThat(size("en", 15_000L)).isEqualTo("14.6 KB")
        // A trailing zero is not a fraction digit worth printing, as docFileSize
        // and the web's formatFileSize both decide.
        assertThat(size("ru", 2L shl 20)).isEqualTo("2 МБ")
    }

    @Test
    fun `whole bytes never carry a fraction`() {
        assertThat(size("ru", 0L)).isEqualTo("0 Б")
        assertThat(size("ru", 512L)).isEqualTo("512 Б")
        assertThat(size("en", 512L)).isEqualTo("512 B")
    }

    @Test
    fun `the unit switches where the thousand does`() {
        // 1024, not 1000 — the server reports bytes and every client scales by
        // the binary step (`docFileSize`, `confFileSize`, web `formatFileSize`).
        // Four digits are grouped the way the locale groups them, and in Russian
        // that separator is a no-break space (U+00A0) both here and in `Intl`.
        assertThat(size("ru", 1023L)).isEqualTo("1\u00A0023 Б")
        assertThat(size("ru", 1024L)).isEqualTo("1 КБ")
        assertThat(size("ru", (1L shl 20) - 1)).isEqualTo("1\u00A0024 КБ")
        assertThat(size("ru", 1L shl 20)).isEqualTo("1 МБ")
    }

    @Test
    fun `a size the provider could not report is not a negative file`() {
        // The pickers pass `-1` through as «let the server decide»; if one ever
        // reaches the row, «-0,0 Б» is not what it should say.
        assertThat(size("ru", -1L)).isEqualTo("0 Б")
    }
}
