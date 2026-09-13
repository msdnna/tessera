package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

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
class UploadsTest {
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
}
