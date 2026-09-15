package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import java.util.Locale
import org.junit.Test
import website.msdnna.tessera.data.model.ConferenceMessage
import website.msdnna.tessera.ui.viewmodels.ConferenceChatUiState

/**
 * The in-call chat's rules (#2896 §7).
 *
 * Everything the sheet decides lives here rather than in the composable, and the
 * reason is this file: merging a page into a list, counting what a reader has
 * not seen and refusing a file the server would refuse are all answerable
 * without a screen, a socket or a call.
 */
class ConferenceChatTest {
    private fun msg(
        id: String,
        at: String,
        userId: String? = "u1",
        body: String = id,
    ) = ConferenceMessage(id = id, userId = userId, userName = "N", body = body, createdAt = at)

    private fun file(name: String, size: Long) = ConfPendingFile(uri = "content://$name", name = name, size = size)

    // ── picking files ─────────────────────────────────────────────────────

    @Test
    fun `accepts files under the caps`() {
        val result = confChatAccept(emptyList(), listOf(file("a.txt", 10), file("b.txt", 20)))

        assertThat(result.pending.map { it.name }).containsExactly("a.txt", "b.txt").inOrder()
        assertThat(result.refusal).isEqualTo(ConfPickRefusal.NONE)
    }

    @Test
    fun `an oversized file is dropped and named while the rest go through`() {
        val result = confChatAccept(
            emptyList(),
            listOf(file("small.txt", 10), file("huge.mp4", CONF_CHAT_MAX_BYTES + 1), file("also.txt", 20)),
        )

        assertThat(result.pending.map { it.name }).containsExactly("small.txt", "also.txt").inOrder()
        assertThat(result.refusal).isEqualTo(ConfPickRefusal.TOO_BIG)
        // Without the name, «файл больше 25 МБ» is unanswerable when five were picked.
        assertThat(result.refusedName).isEqualTo("huge.mp4")
    }

    @Test
    fun `the cap stops the pick and the extra files are not counted twice`() {
        val already = (1..CONF_CHAT_MAX_FILES).map { file("have$it", 1) }

        val result = confChatAccept(already, listOf(file("more.txt", 1), file("evenmore.txt", 1)))

        assertThat(result.pending).hasSize(CONF_CHAT_MAX_FILES)
        assertThat(result.refusal).isEqualTo(ConfPickRefusal.TOO_MANY)
        assertThat(result.refusedName).isEqualTo("more.txt")
    }

    @Test
    fun `the first refusal is the one reported`() {
        val picked = listOf(file("huge.mp4", CONF_CHAT_MAX_BYTES + 1)) +
            (1..CONF_CHAT_MAX_FILES).map { file("ok$it", 1) } +
            file("late.txt", 1)

        val result = confChatAccept(emptyList(), picked)

        // The size refusal happened first; overwriting it with TOO_MANY would
        // report the harmless file and hide the one that was actually lost.
        assertThat(result.refusal).isEqualTo(ConfPickRefusal.TOO_BIG)
        assertThat(result.refusedName).isEqualTo("huge.mp4")
        assertThat(result.pending).hasSize(CONF_CHAT_MAX_FILES)
    }

    // The rule that bounds the *request* rather than a file. CONF_CHAT_MAX_FILES
    // says how many may travel, never how much: five 20 MB videos passed both
    // checks here and on the web, and the server refused the batch — for most of
    // this route's life not even with its own sentence, because the route carried
    // no body budget and the transport answered first with "request body too
    // large", which names nothing the user picked (#2932).
    @Test
    fun `a batch over the total cap is refused, and what fits is kept`() {
        val ten = 10L * 1024 * 1024

        val result = confChatAccept(
            emptyList(),
            listOf(file("а.mp4", ten), file("б.mp4", ten), file("в.mp4", ten)),
        )

        assertThat(result.pending.map { it.name }).containsExactly("а.mp4", "б.mp4").inOrder()
        assertThat(result.refusal).isEqualTo(ConfPickRefusal.TOO_BIG_TOTAL)
        assertThat(result.refusedName).isEqualTo("в.mp4")
    }

    @Test
    fun `the total counts what is already queued`() {
        val already = listOf(file("снято.mp4", CONF_CHAT_MAX_TOTAL_BYTES - 1024))

        val result = confChatAccept(already, listOf(file("ещё.png", 2 * 1024)))

        assertThat(result.pending).hasSize(1)
        assertThat(result.refusal).isEqualTo(ConfPickRefusal.TOO_BIG_TOTAL)
    }

    @Test
    fun `a batch landing exactly on the total cap goes through`() {
        val half = CONF_CHAT_MAX_TOTAL_BYTES / 2

        val result = confChatAccept(emptyList(), listOf(file("a", half), file("b", CONF_CHAT_MAX_TOTAL_BYTES - half)))

        assertThat(result.pending).hasSize(2)
        assertThat(result.refusal).isEqualTo(ConfPickRefusal.NONE)
    }

    // ── the composer ──────────────────────────────────────────────────────

    @Test
    fun `body length counts code points, not UTF-16 units`() {
        // An emoji is two chars to Kotlin and one rune to Go. Counting the
        // Kotlin way would refuse a message the server accepts.
        assertThat("🙂".length).isEqualTo(2)
        assertThat(confChatBodyLength("🙂")).isEqualTo(1)
        assertThat(confChatBodyLength("  привет  ")).isEqualTo(6)
    }

    @Test
    fun `send is off for an empty composer and on for files alone`() {
        assertThat(confChatCanSend("", emptyList(), sending = false)).isFalse()
        assertThat(confChatCanSend("   ", emptyList(), sending = false)).isFalse()
        assertThat(confChatCanSend("", listOf(file("a.txt", 1)), sending = false)).isTrue()
        assertThat(confChatCanSend("hi", emptyList(), sending = false)).isTrue()
    }

    @Test
    fun `send is off while sending and over the body cap`() {
        assertThat(confChatCanSend("hi", emptyList(), sending = true)).isFalse()
        assertThat(confChatCanSend("x".repeat(CONF_CHAT_MAX_BODY), emptyList(), sending = false)).isTrue()
        assertThat(confChatCanSend("x".repeat(CONF_CHAT_MAX_BODY + 1), emptyList(), sending = false)).isFalse()
    }

    // ── removing a line ───────────────────────────────────────────────────

    @Test
    fun `everyone may remove their own line and nobody else's`() {
        val mine = msg("m1", "2026-09-05T10:00:00Z", userId = "me")
        val theirs = msg("m2", "2026-09-05T10:01:00Z", userId = "them")

        assertThat(confChatCanRemove(mine, meId = "me", canModerate = false)).isTrue()
        assertThat(confChatCanRemove(theirs, meId = "me", canModerate = false)).isFalse()
        assertThat(confChatCanRemove(theirs, meId = "me", canModerate = true)).isTrue()
    }

    @Test
    fun `a line whose author is gone belongs to nobody`() {
        val orphan = msg("m1", "2026-09-05T10:00:00Z", userId = null)

        assertThat(confChatCanRemove(orphan, meId = "me", canModerate = false)).isFalse()
        assertThat(confChatCanRemove(orphan, meId = "me", canModerate = true)).isTrue()
        // An unknown «me» must not match an unknown author into a delete button.
        assertThat(confChatCanRemove(msg("m2", "t", userId = ""), meId = "", canModerate = false)).isFalse()
    }

    // ── folding a page into the list ──────────────────────────────────────

    @Test
    fun `an empty page means an empty chat`() {
        val merge = confChatMergeTail(listOf(msg("a", "1")), emptyList())

        assertThat(merge.messages).isEmpty()
        assertThat(merge.stitched).isFalse()
    }

    @Test
    fun `an overlapping page keeps the history above it`() {
        val current = listOf(msg("old", "2026-09-05T09:00:00Z"), msg("b", "2026-09-05T10:00:00Z"))
        val tail = listOf(msg("b", "2026-09-05T10:00:00Z"), msg("c", "2026-09-05T10:01:00Z"))

        val merge = confChatMergeTail(current, tail)

        assertThat(merge.messages.map { it.id }).containsExactly("old", "b", "c").inOrder()
        assertThat(merge.stitched).isTrue()
    }

    @Test
    fun `a line the page no longer carries is gone from the window`() {
        val current = listOf(
            msg("old", "2026-09-05T09:00:00Z"),
            msg("b", "2026-09-05T10:00:00Z"),
            msg("deleted", "2026-09-05T10:00:30Z"),
        )
        val tail = listOf(msg("b", "2026-09-05T10:00:00Z"), msg("c", "2026-09-05T10:01:00Z"))

        val merge = confChatMergeTail(current, tail)

        // Inside the page's own window the page is authoritative — a union would
        // leave a removed message on the phone of everyone who had the rail open.
        assertThat(merge.messages.map { it.id }).containsExactly("old", "b", "c").inOrder()
    }

    @Test
    fun `a page that does not touch what we hold replaces it`() {
        val current = listOf(msg("a", "2026-09-05T09:00:00Z"))
        val tail = listOf(msg("y", "2026-09-05T12:00:00Z"), msg("z", "2026-09-05T12:01:00Z"))

        val merge = confChatMergeTail(current, tail)

        // Keeping both halves would paint a conversation with a silent hole in
        // the middle, where the missing lines are the ones nobody can ask for.
        assertThat(merge.messages.map { it.id }).containsExactly("y", "z").inOrder()
        assertThat(merge.stitched).isFalse()
    }

    @Test
    fun `our own optimistic line is not doubled by the page carrying it`() {
        val sentAt = "2026-09-05T10:01:00Z"
        val current = listOf(msg("b", "2026-09-05T10:00:00Z"), msg("mine", sentAt))
        val tail = listOf(msg("b", "2026-09-05T10:00:00Z"), msg("mine", sentAt))

        val merge = confChatMergeTail(current, tail)

        assertThat(merge.messages.map { it.id }).containsExactly("b", "mine").inOrder()
    }

    @Test
    fun `older pages are prepended without duplicates`() {
        val current = listOf(msg("b", "2"), msg("c", "3"))
        val older = listOf(msg("a", "1"), msg("b", "2"))

        assertThat(confChatPrepend(older, current).map { it.id }).containsExactly("a", "b", "c").inOrder()
        assertThat(confChatPrepend(emptyList(), current).map { it.id }).containsExactly("b", "c").inOrder()
    }

    // ── the unread badge ──────────────────────────────────────────────────

    @Test
    fun `unread counts what arrived after the anchor, minus our own`() {
        val messages = listOf(
            msg("a", "1", userId = "them"),
            msg("seen", "2", userId = "them"),
            msg("mine", "3", userId = "me"),
            msg("new", "4", userId = "them"),
        )

        // Our own message nudges the room too — counting nudges would badge the
        // chat for something we just said ourselves.
        assertThat(confChatUnread(messages, lastSeenId = "seen", meId = "me")).isEqualTo(1)
        assertThat(confChatUnread(messages, lastSeenId = "new", meId = "me")).isEqualTo(0)
        assertThat(confChatUnread(emptyList(), lastSeenId = "", meId = "me")).isEqualTo(0)
    }

    @Test
    fun `an unknown anchor means everything is unread`() {
        val messages = listOf(msg("a", "1", userId = "them"), msg("b", "2", userId = null))

        // Including the orphan row: a message whose author is gone was still not
        // written by us.
        assertThat(confChatUnread(messages, lastSeenId = "", meId = "me")).isEqualTo(2)
    }

    @Test
    fun `an open sheet never shows a badge`() {
        val messages = listOf(msg("a", "1", userId = "them"))
        val state = ConferenceChatUiState(messages = messages, meId = "me", lastSeenId = "")

        assertThat(state.copy(open = false).unread).isEqualTo(1)
        assertThat(state.copy(open = true).unread).isEqualTo(0)
    }

    @Test
    fun `the state's delete rule follows the moderator flag`() {
        val theirs = msg("m", "1", userId = "them")
        val state = ConferenceChatUiState(messages = listOf(theirs), meId = "me")

        assertThat(state.canRemove(theirs)).isFalse()
        assertThat(state.copy(canModerate = true).canRemove(theirs)).isTrue()
    }

    // ── file sizes ────────────────────────────────────────────────────────

    @Test
    fun `sizes split into a number and a localisable unit`() {
        assertThat(confFileSize(512, Locale.US)).isEqualTo(ConfFileSize("512", ConfSizeUnit.BYTES))
        assertThat(confFileSize(2048, Locale.US)).isEqualTo(ConfFileSize("2", ConfSizeUnit.KB))
        // Three whole megabytes are «3 МБ», not «3.0 МБ» — the fraction digit is a
        // maximum, the way `Intl.NumberFormat` spends it on the web rail.
        assertThat(confFileSize(3L * 1024 * 1024, Locale.US))
            .isEqualTo(ConfFileSize("3", ConfSizeUnit.MB))
        assertThat(confFileSize(1536L * 1024, Locale.US).amount).isEqualTo("1.5")
        assertThat(confFileSize(-1, Locale.US)).isEqualTo(ConfFileSize("0", ConfSizeUnit.BYTES))
    }

    @Test
    fun `the number is written in the reader's language, like every other file size`() {
        // The unit comes from the string catalogue and the separator from the
        // reader's own formatter — both halves are locale business. Formatting the
        // number in `Locale.US` while the unit said «МБ» is what made one
        // attachment read «1.4 МБ» in a call and «1,4 МБ» in a document, inside the
        // same Russian interface (web `formatFileSize` and phone `docFileSize` both
        // localise it).
        val ru = Locale.forLanguageTag("ru-RU")
        assertThat(confFileSize(1536L * 1024, ru).amount).isEqualTo("1,5")
        assertThat(confFileSize(14_950, ru).amount).isEqualTo("14,6")
        assertThat(confFileSize(1536L * 1024, Locale.UK).amount).isEqualTo("1.5")
    }

    @Test
    fun `kilobytes keep the fraction digit the web rail keeps`() {
        // Rounding to whole kilobytes was the phone's own rule: the same 14 950-byte
        // file reads «14.6 KB» on the web and used to read «15 KB» here.
        assertThat(confFileSize(14_950, Locale.US).amount).isEqualTo("14.6")
    }
}
