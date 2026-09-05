package website.msdnna.tessera.ui.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.data.model.ConferenceAttachment
import website.msdnna.tessera.data.model.ConferenceMessage
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.theme.TesseraTheme
import website.msdnna.tessera.ui.viewmodels.ConferenceChatUiState
import website.msdnna.tessera.util.ConfPendingFile
import website.msdnna.tessera.util.ConfPickRefusal

/**
 * The in-call chat sheet (#2896 §7), drawn against a state rather than a call.
 *
 * The rules it draws are specced in `util/ConferenceChatTest`; what is left here
 * is what only a rendered sheet can answer — that a delete button appears on the
 * lines this phone may remove and nowhere else, that pressing it reaches the
 * server only through the confirmation, and that a refused pick says so.
 *
 * Screen size pinned as in [ConferenceRoomTest]: the default 320×470px drops a
 * tap below the edge in silence.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class ConferenceChatSheetTest {
    @get:Rule
    val compose = createComposeRule()

    private fun msg(
        id: String,
        userId: String? = "them",
        body: String = "текст $id",
        attachments: List<ConferenceAttachment> = emptyList(),
    ) = ConferenceMessage(
        id = id,
        userId = userId,
        userName = "Кто-то",
        body = body,
        createdAt = "2026-09-05T10:00:00Z",
        attachments = attachments,
    )

    private var closed = 0
    private var older = 0
    private var removeAsked = ""
    private var removeConfirmed = 0
    private var removeCancelled = 0
    private var refusalDismissed = 0
    private var downloaded = ""
    private var sent = 0
    private var dropped = ""

    private fun mount(state: ConferenceChatUiState) {
        compose.setContent {
            TesseraTheme {
                ConferenceChatSheet(
                    state = state,
                    onClose = { closed += 1 },
                    onDraft = {},
                    onSend = { sent += 1 },
                    onAttach = {},
                    onDropPending = { dropped = it },
                    onDismissRefusal = { refusalDismissed += 1 },
                    onLoadOlder = { older += 1 },
                    onAskRemove = { removeAsked = it },
                    onCancelRemove = { removeCancelled += 1 },
                    onConfirmRemove = { removeConfirmed += 1 },
                    onDownload = { downloaded = it.id },
                    onDismissError = {},
                )
            }
        }
    }

    @Test
    fun `a chat nobody has written in shows the empty line and no log`() {
        mount(ConferenceChatUiState(open = true))

        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_EMPTY).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_LOG).assertDoesNotExist()
    }

    @Test
    fun `the delete button is only on the lines this phone may remove`() {
        mount(
            ConferenceChatUiState(
                open = true,
                meId = "me",
                messages = listOf(msg("mine", userId = "me"), msg("theirs")),
            ),
        )

        compose.onNodeWithTag(TestTags.conferenceChatDelete("mine")).assertIsDisplayed()
        // Not a moderator: somebody else's line is readable, not removable.
        compose.onNodeWithTag(TestTags.conferenceChatDelete("theirs")).assertDoesNotExist()
    }

    @Test
    fun `a moderator may remove anyone's line`() {
        mount(
            ConferenceChatUiState(
                open = true,
                meId = "me",
                canModerate = true,
                messages = listOf(msg("theirs")),
            ),
        )

        compose.onNodeWithTag(TestTags.conferenceChatDelete("theirs")).assertIsDisplayed()
    }

    @Test
    fun `pressing delete asks rather than removes`() {
        mount(ConferenceChatUiState(open = true, meId = "me", messages = listOf(msg("mine", userId = "me"))))

        compose.onNodeWithTag(TestTags.conferenceChatDelete("mine")).performClick()

        // The press names the line for the dialog and reaches nothing else:
        // removal is visible to the whole room and cannot be taken back.
        assertThat(removeAsked).isEqualTo("mine")
        assertThat(removeConfirmed).isEqualTo(0)
        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_DELETE_CONFIRM).assertDoesNotExist()
    }

    @Test
    fun `only the dialog's confirm reaches the server`() {
        mount(
            ConferenceChatUiState(
                open = true,
                meId = "me",
                messages = listOf(msg("mine", userId = "me")),
                removing = "mine",
            ),
        )

        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_DELETE_CONFIRM).performClick()

        assertThat(removeConfirmed).isEqualTo(1)
        assertThat(removeCancelled).isEqualTo(0)
    }

    @Test
    fun `a refused pick is named and dismissed by tapping it`() {
        mount(
            ConferenceChatUiState(
                open = true,
                refusal = ConfPickRefusal.TOO_BIG,
                refusedName = "huge.mp4",
            ),
        )

        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_REFUSAL).performClick()

        assertThat(refusalDismissed).isEqualTo(1)
    }

    @Test
    fun `a pick that lost nothing carries no refusal line`() {
        mount(ConferenceChatUiState(open = true, pending = listOf(pending("a.txt"))))

        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_REFUSAL).assertDoesNotExist()
        compose.onNodeWithTag(TestTags.conferenceChatPending("content://a.txt")).assertIsDisplayed()
    }

    @Test
    fun `an unsent file can be taken back off the message`() {
        mount(ConferenceChatUiState(open = true, pending = listOf(pending("a.txt"), pending("b.txt"))))

        compose.onNodeWithTag(TestTags.conferenceChatPendingRemove("content://b.txt")).performClick()

        // By uri, not by position: dropping the second of two files must not take
        // the first one off instead, and the picker offers no way back.
        assertThat(dropped).isEqualTo("content://b.txt")
    }

    @Test
    fun `the older link is there only while there is older text`() {
        val messages = listOf(msg("a"))
        mount(ConferenceChatUiState(open = true, messages = messages, hasMore = true))

        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_OLDER).performClick()
        assertThat(older).isEqualTo(1)
    }

    @Test
    fun `a chat showing everything it has does not offer earlier text`() {
        mount(ConferenceChatUiState(open = true, messages = listOf(msg("a")), hasMore = false))

        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_OLDER).assertDoesNotExist()
    }

    @Test
    fun `send does nothing while the composer is empty`() {
        mount(ConferenceChatUiState(open = true))

        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_SEND).performClick()

        // The button stays on screen and stays inert — a disabled-looking control
        // that still posts an empty message is the failure this guards.
        assertThat(sent).isEqualTo(0)
    }

    @Test
    fun `send works once there is something to send`() {
        mount(ConferenceChatUiState(open = true, draft = "привет"))

        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_SEND).performClick()

        assertThat(sent).isEqualTo(1)
    }

    @Test
    fun `an over-long draft cannot be sent`() {
        mount(ConferenceChatUiState(open = true, draft = "x".repeat(4001)))

        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_SEND).performClick()

        assertThat(sent).isEqualTo(0)
    }

    @Test
    fun `tapping an attachment asks for that file`() {
        val file = ConferenceAttachment(id = "att1", filename = "log.txt", type = "text/plain", size = 2048)
        mount(ConferenceChatUiState(open = true, messages = listOf(msg("a", attachments = listOf(file)))))

        compose.onNodeWithTag(TestTags.conferenceChatAttachment("att1")).performClick()

        assertThat(downloaded).isEqualTo("att1")
    }

    @Test
    fun `the close control closes the sheet`() {
        mount(ConferenceChatUiState(open = true))

        compose.onNodeWithTag(TestTags.CONFERENCE_CHAT_CLOSE).performClick()

        assertThat(closed).isEqualTo(1)
    }

    private fun pending(name: String) = ConfPendingFile(uri = "content://$name", name = name, size = 10)
}
