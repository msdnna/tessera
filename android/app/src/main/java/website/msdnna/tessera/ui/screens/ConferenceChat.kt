package website.msdnna.tessera.ui.screens

import android.content.ContentResolver
import android.net.Uri
import android.provider.OpenableColumns
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import java.io.File
import java.io.IOException
import okhttp3.MediaType.Companion.toMediaTypeOrNull
import okhttp3.MultipartBody
import okhttp3.RequestBody
import okio.BufferedSink
import okio.source
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.ConferenceAttachment
import website.msdnna.tessera.data.model.ConferenceMessage
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.MemberAvatar
import website.msdnna.tessera.ui.components.TConfirmDialog
import website.msdnna.tessera.ui.components.TFormError
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.LocalDateFormat
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.ConfUploadSource
import website.msdnna.tessera.ui.viewmodels.ConferenceChatUiState
import website.msdnna.tessera.ui.viewmodels.ConferenceChatViewModel
import website.msdnna.tessera.util.CONF_CHAT_MAX_BODY
import website.msdnna.tessera.util.CONF_CHAT_MAX_FILES
import website.msdnna.tessera.util.ConfPendingFile
import website.msdnna.tessera.util.ConfPickRefusal
import website.msdnna.tessera.util.ConfSizeUnit
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.confChatBodyLength
import website.msdnna.tessera.util.confFileSize
import website.msdnna.tessera.util.localTimeLabel

/**
 * The in-call chat (#2896 §7, web `ConferenceChat.vue`) with everything around it
 * that needs Android: the file picker, the upload source and the system viewer.
 *
 * Split from [ConferenceChatSheet] on that line exactly. Below this function the
 * sheet is a pure drawing of a state, which is what lets the room's own spec
 * mount a call with a chat in it; above it lives the part that cannot exist
 * without a `ContentResolver`.
 */
@Composable
internal fun ConferenceChatHost(vm: ConferenceChatViewModel, state: ConferenceChatUiState) {
    val ctx = LocalContext.current
    val chooserTitle = stringResource(R.string.conf_chat_download)

    // OpenMultipleDocuments rather than GetContent: five files is the server's
    // cap and picking them one dialog at a time is four dialogs of the user's
    // patience. It also hands back a URI we may read, which GetContent's
    // does not always survive a process death to do.
    val picker = rememberLauncherForActivityResult(
        ActivityResultContracts.OpenMultipleDocuments(),
    ) { uris -> vm.attach(uris.orEmpty().mapNotNull { pendingFile(ctx.contentResolver, it) }) }

    val source = remember(ctx) { ResolverUploadSource(ctx.contentResolver) }

    ConferenceChatSheet(
        state = state,
        onClose = { vm.close() },
        onDraft = { vm.setDraft(it) },
        onSend = { vm.send(source) },
        // Everything, filtered by the server rather than by us: the chat carries
        // logs and archives as often as it carries screenshots, and a MIME filter
        // is how «почему я не могу отправить дамп» starts.
        onAttach = { picker.launch(arrayOf("*/*")) },
        onDropPending = { vm.dropPending(it) },
        onDismissRefusal = { vm.dismissRefusal() },
        onLoadOlder = { vm.loadOlder() },
        onAskRemove = { vm.askRemove(it) },
        onCancelRemove = { vm.cancelRemove() },
        onConfirmRemove = { vm.confirmRemove() },
        onDownload = { attachment ->
            vm.download(ctx.cacheDir, attachment.id, attachment.filename) { file ->
                openDownloadedFile(ctx, file, attachment.type, chooserTitle)
            }
        },
        onDismissError = { vm.clearError() },
    )
}

/**
 * What the picker gave us, as the rules see it.
 *
 * A URI whose name and size cannot be read is dropped instead of guessed at: an
 * unnamed file of unknown length would skip the 25 MB check and fail on the
 * server after the upload, which is the one failure this screen exists to avoid.
 */
private fun pendingFile(resolver: ContentResolver, uri: Uri): ConfPendingFile? = runCatching {
    resolver.query(uri, null, null, null, null)?.use { cursor ->
        if (!cursor.moveToFirst()) return@use null
        val nameIdx = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
        val sizeIdx = cursor.getColumnIndex(OpenableColumns.SIZE)
        val name = if (nameIdx >= 0) cursor.getString(nameIdx) else null
        if (name.isNullOrBlank() || sizeIdx < 0 || cursor.isNull(sizeIdx)) return@use null
        ConfPendingFile(
            uri = uri.toString(),
            name = name,
            size = cursor.getLong(sizeIdx),
            mime = resolver.getType(uri),
        )
    }
}.getOrNull()

/**
 * Streams a picked file into the request instead of reading it into memory.
 *
 * Five attachments at the 25 MB cap is 125 MB, and a phone in a video call has
 * neither that heap nor a reason to hold one — the bytes are wanted once, in
 * order, by a socket.
 */
private class ResolverUploadSource(private val resolver: ContentResolver) : ConfUploadSource {
    override fun part(file: ConfPendingFile): MultipartBody.Part? {
        val uri = runCatching { Uri.parse(file.uri) }.getOrNull() ?: return null
        // Opened once here to find out whether it can be opened at all. A part
        // that fails inside `writeTo` fails mid-request, and the message it was
        // attached to is lost with it.
        val readable = runCatching { resolver.openInputStream(uri)?.use { true } }.getOrNull() ?: return null
        if (!readable) return null
        return MultipartBody.Part.createFormData("files", file.name, ResolverBody(resolver, uri, file.mime, file.size))
    }
}

private class ResolverBody(
    private val resolver: ContentResolver,
    private val uri: Uri,
    private val mime: String?,
    private val size: Long,
) : RequestBody() {
    override fun contentType() = (mime ?: OCTET_STREAM).toMediaTypeOrNull()

    /** Known up front, so the upload is not chunked — a progress bar on the other
     *  side has nothing to show without a length. */
    override fun contentLength(): Long = size

    override fun writeTo(sink: BufferedSink) {
        val input = resolver.openInputStream(uri) ?: throw IOException("cannot read $uri")
        input.use { sink.writeAll(it.source()) }
    }

    private companion object {
        const val OCTET_STREAM = "application/octet-stream"
    }
}

/**
 * The chat as a bottom sheet, driven purely by [state].
 *
 * A sheet over the call rather than the web's side rail, for the reason the
 * roster gives: a phone has no width beside a video stream. It opens over half
 * the screen and pulls up to the whole of it — see [ConferenceSheet] — so the
 * common case, reading a line without leaving the meeting, costs nothing. The
 * unread badge still lives on the toolbar button rather than in here: at full
 * height the chat does cover the call, and that is the case the badge is for.
 */
@Composable
internal fun ConferenceChatSheet(
    state: ConferenceChatUiState,
    onClose: () -> Unit,
    onDraft: (String) -> Unit,
    onSend: () -> Unit,
    onAttach: () -> Unit,
    onDropPending: (String) -> Unit,
    onDismissRefusal: () -> Unit,
    onLoadOlder: () -> Unit,
    onAskRemove: (String) -> Unit,
    onCancelRemove: () -> Unit,
    onConfirmRemove: () -> Unit,
    onDownload: (ConferenceAttachment) -> Unit,
    onDismissError: () -> Unit,
) {
    val c = Tessera.colors
    val log = rememberLazyListState()

    // New text arrives at the bottom, which is where a reader who is at the
    // bottom expects to stay. Keyed on the newest id rather than on the size:
    // paging older messages in grows the list too, and jumping to the end then
    // would throw the reader out of the history they just asked for.
    LaunchedEffect(state.messages.lastOrNull()?.id) {
        if (state.messages.isNotEmpty()) log.scrollToItem(state.messages.lastIndex)
    }

    ConferenceSheet(
        tag = TestTags.CONFERENCE_CHAT,
        handleTag = TestTags.CONFERENCE_CHAT_HANDLE,
        handleLabel = stringResource(R.string.conf_sheet_expand),
        onClose = onClose,
        header = { dismiss ->
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    stringResource(R.string.conf_chat_title),
                    color = c.text1,
                    fontSize = 15.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.weight(1f),
                )
                IonIcon(
                    Ion.CLOSE,
                    size = 18.dp,
                    tint = c.text2,
                    description = stringResource(R.string.conf_chat_close),
                    modifier = Modifier.clickableNoRipple(onClick = dismiss)
                        .testTag(TestTags.CONFERENCE_CHAT_CLOSE),
                )
            }
        },
    ) {
        if (state.messages.isEmpty()) {
            Text(
                stringResource(R.string.conf_chat_empty),
                color = c.text3,
                fontSize = 13.sp,
                modifier = Modifier.fillMaxWidth().weight(1f).padding(vertical = 20.dp)
                    .testTag(TestTags.CONFERENCE_CHAT_EMPTY),
            )
        } else {
            LazyColumn(
                Modifier.fillMaxWidth().weight(1f).testTag(TestTags.CONFERENCE_CHAT_LOG),
                state = log,
            ) {
                // Inside the log rather than above it: «более ранние» belongs
                // at the top of the history it prepends to, and there it
                // scrolls away instead of taking a strip of a phone screen
                // for the rest of the call.
                if (state.hasMore) {
                    item(key = "older") {
                        Text(
                            stringResource(R.string.conf_chat_load_older),
                            color = c.primary,
                            fontSize = 12.sp,
                            fontWeight = FontWeight.Medium,
                            modifier = Modifier.fillMaxWidth()
                                .clickableNoRipple(onClick = onLoadOlder)
                                .padding(horizontal = 14.dp, vertical = 8.dp)
                                .testTag(TestTags.CONFERENCE_CHAT_OLDER),
                        )
                    }
                }
                items(state.messages.size, key = { state.messages[it].id }) { i ->
                    val message = state.messages[i]
                    MessageRow(
                        message = message,
                        removable = state.canRemove(message),
                        onAskRemove = { onAskRemove(message.id) },
                        onDownload = onDownload,
                    )
                }
            }
        }

        if (state.refusal != ConfPickRefusal.NONE) {
            // Dismissed by tapping the line itself: a wrapper with the click
            // on it would swallow the tag the spec selects by.
            Text(
                refusalText(state.refusal, state.refusedName),
                color = c.text2,
                fontSize = 12.sp,
                modifier = Modifier.fillMaxWidth().background(c.surfaceAlt)
                    .clickableNoRipple(onClick = onDismissRefusal)
                    .padding(horizontal = 14.dp, vertical = 8.dp)
                    .testTag(TestTags.CONFERENCE_CHAT_REFUSAL),
            )
        }

        if (state.error != null) {
            TFormError(
                state.error,
                modifier = Modifier.clickableNoRipple(onClick = onDismissError)
                    .padding(horizontal = 14.dp, vertical = 4.dp),
            )
        }

        Composer(
            state = state,
            onDraft = onDraft,
            onSend = onSend,
            onAttach = onAttach,
            onDropPending = onDropPending,
        )
    }

    if (state.removing.isNotBlank()) {
        TConfirmDialog(
            title = stringResource(R.string.conf_chat_delete),
            message = stringResource(R.string.conf_chat_delete_confirm),
            confirmTag = TestTags.CONFERENCE_CHAT_DELETE_CONFIRM,
            onConfirm = onConfirmRemove,
            onDismiss = onCancelRemove,
        )
    }
}

@Composable
private fun MessageRow(
    message: ConferenceMessage,
    removable: Boolean,
    onAskRemove: () -> Unit,
    onDownload: (ConferenceAttachment) -> Unit,
) {
    val c = Tessera.colors
    // A row whose author left the workspace: the message outlives the account,
    // and «» over an avatar would read as a rendering failure.
    val name = message.userName?.takeIf { it.isNotBlank() }
        ?: stringResource(R.string.conf_chat_unknown_author)
    val at = localTimeLabel(LocalResources.current, message.createdAt, LocalDateFormat.current)

    Row(
        Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 6.dp)
            .testTag(TestTags.conferenceChatMessage(message.id)),
    ) {
        MemberAvatar(26.dp, name, userId = message.userId.orEmpty())
        Spacer(Modifier.width(9.dp))
        Column(Modifier.weight(1f)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(name, color = c.text1, fontSize = 12.sp, fontWeight = FontWeight.Medium, maxLines = 1)
                Spacer(Modifier.width(6.dp))
                Text(at, color = c.text3, fontSize = 11.sp)
            }
            if (message.body.isNotBlank()) {
                Text(message.body, color = c.text2, fontSize = 13.sp)
            }
            message.attachments.forEach { attachment ->
                AttachmentRow(attachment, onClick = { onDownload(attachment) })
            }
        }
        if (removable) {
            Spacer(Modifier.width(6.dp))
            IonIcon(
                Ion.TRASH,
                size = 15.dp,
                tint = c.text3,
                description = stringResource(R.string.conf_chat_delete),
                modifier = Modifier.clip(CircleShape)
                    .clickableNoRipple(onClick = onAskRemove)
                    .testTag(TestTags.conferenceChatDelete(message.id)),
            )
        }
    }
}

/**
 * One file on a message.
 *
 * Downloaded on tap even when it is an image: rendering it inline would mean
 * fetching every picture in the history over the same connection the call is
 * using, and a call that stutters because somebody scrolled the chat is a worse
 * trade than a tap.
 */
@Composable
private fun AttachmentRow(attachment: ConferenceAttachment, onClick: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().padding(top = 4.dp)
            .clickableNoRipple(onClick = onClick)
            .testTag(TestTags.conferenceChatAttachment(attachment.id)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(if (attachment.isImage) Ion.IMAGE else Ion.DOCUMENT_TEXT, size = 14.dp, tint = c.text3)
        Spacer(Modifier.width(6.dp))
        Text(attachment.filename, color = c.primary, fontSize = 12.sp, maxLines = 1, modifier = Modifier.weight(1f))
        Spacer(Modifier.width(6.dp))
        Text(fileSizeLabel(attachment.size), color = c.text3, fontSize = 11.sp)
    }
}

@Composable
private fun Composer(
    state: ConferenceChatUiState,
    onDraft: (String) -> Unit,
    onSend: () -> Unit,
    onAttach: () -> Unit,
    onDropPending: (String) -> Unit,
) {
    val c = Tessera.colors

    state.pending.forEach { file ->
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 2.dp)
                .testTag(TestTags.conferenceChatPending(file.uri)),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IonIcon(Ion.DOCUMENT_TEXT, size = 14.dp, tint = c.text3)
            Spacer(Modifier.width(6.dp))
            Text(file.name, color = c.text2, fontSize = 12.sp, maxLines = 1, modifier = Modifier.weight(1f))
            Spacer(Modifier.width(6.dp))
            Text(fileSizeLabel(file.size), color = c.text3, fontSize = 11.sp)
            Spacer(Modifier.width(8.dp))
            IonIcon(
                Ion.CLOSE,
                size = 14.dp,
                tint = c.text3,
                description = stringResource(R.string.conf_chat_remove_file),
                modifier = Modifier.clickableNoRipple { onDropPending(file.uri) }
                    .testTag(TestTags.conferenceChatPendingRemove(file.uri)),
            )
        }
    }

    // Said before the press rather than after it: the server's answer to an
    // over-long message is a 400, and by then the text has already been typed.
    val over = confChatBodyLength(state.draft) > CONF_CHAT_MAX_BODY
    if (over) {
        TFormError(
            stringResource(R.string.conf_chat_too_long, CONF_CHAT_MAX_BODY),
            modifier = Modifier.padding(horizontal = 14.dp),
        )
    }

    Row(
        Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(
            Ion.ATTACH,
            size = 19.dp,
            tint = if (state.pending.size < CONF_CHAT_MAX_FILES) c.text2 else c.text3,
            description = stringResource(R.string.conf_chat_attach),
            modifier = Modifier.clip(CircleShape)
                .clickableNoRipple(onClick = onAttach)
                .testTag(TestTags.CONFERENCE_CHAT_ATTACH),
        )
        Spacer(Modifier.width(8.dp))
        TTextField(
            value = state.draft,
            onValueChange = onDraft,
            placeholder = stringResource(R.string.conf_chat_placeholder),
            singleLine = false,
            modifier = Modifier.weight(1f),
            fieldTag = TestTags.CONFERENCE_CHAT_INPUT,
        )
        Spacer(Modifier.width(8.dp))
        IonIcon(
            Ion.SEND,
            size = 19.dp,
            tint = if (state.canSend) c.primary else c.text3,
            gradient = state.canSend,
            description = stringResource(R.string.conf_chat_send),
            modifier = Modifier.clip(CircleShape)
                .clickableNoRipple(enabled = state.canSend, onClick = onSend)
                .testTag(TestTags.CONFERENCE_CHAT_SEND),
        )
    }
}

@Composable
private fun refusalText(refusal: ConfPickRefusal, name: String): String = when (refusal) {
    ConfPickRefusal.TOO_BIG -> stringResource(R.string.conf_chat_file_too_big, name)
    ConfPickRefusal.TOO_MANY -> stringResource(R.string.conf_chat_too_many_files, CONF_CHAT_MAX_FILES)
    ConfPickRefusal.NONE -> ""
}

/** The size split into number and unit by [confFileSize], then localised here —
 *  «КБ» is a translation, `1,4` is not. */
@Composable
private fun fileSizeLabel(bytes: Long): String {
    val size = confFileSize(bytes)
    return stringResource(
        when (size.unit) {
            ConfSizeUnit.MB -> R.string.conf_chat_size_mb
            ConfSizeUnit.KB -> R.string.conf_chat_size_kb
            ConfSizeUnit.BYTES -> R.string.conf_chat_size_b
        },
        size.amount,
    )
}
