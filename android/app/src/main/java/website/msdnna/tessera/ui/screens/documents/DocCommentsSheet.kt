package website.msdnna.tessera.ui.screens.documents

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.DocumentComment
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmDialog
import website.msdnna.tessera.ui.components.TFormError
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.LocalDateFormat
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.DocCommentsState
import website.msdnna.tessera.util.DocThread
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.docCommentAuthor
import website.msdnna.tessera.util.whenLabel

/** What a delete confirmation is about. [replies] words it: dropping a root
 *  takes its answers with it, and that is worth saying before it happens. */
private data class DocRemoveTarget(val id: String, val replies: Int)

/**
 * The way into the discussions, over the reader and over the editor alike, with
 * the number of open ones on it.
 *
 * The count is on the button rather than only inside the sheet because that is
 * the whole point of it: a remark left on a phone-sized document is invisible
 * until something says it is there.
 */
@Composable
fun DocCommentsButton(count: Int, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    Box(modifier) {
        IonIconButton(
            Ion.CHATBUBBLE,
            onClick = onClick,
            boxSize = 40.dp,
            modifier = Modifier.testTag(TestTags.DOCUMENT_COMMENTS_OPEN),
        )
        if (count > 0) {
            Text(
                count.toString(),
                color = c.primary,
                fontSize = 10.sp,
                fontWeight = FontWeight.Bold,
                modifier = Modifier.align(Alignment.TopEnd)
                    .padding(top = 4.dp, end = 2.dp)
                    .testTag(TestTags.DOCUMENT_COMMENTS_BADGE),
            )
        }
    }
}

/**
 * Discussions of the open document (#2730, §5 of #2894).
 *
 * A panel over the document rather than beside it: the web has a column to spare
 * next to the sheet and a phone has none. The consequence is that the curved
 * lines the web draws from a card to its block cannot exist here — so the anchor
 * is *said* instead: the quote the thread was started on, a heading per group,
 * and the block the sheet was opened from first. A thread whose block has since
 * been deleted is shown apart rather than dropped, exactly as on the web.
 *
 * The threads are native even though the text they hang on is a WebView (§4): a
 * conversation is a list of rows, and it has to work over the reader too, where
 * there is no page at all.
 */
@Composable
fun DocCommentsSheet(
    state: DocCommentsState,
    meId: String?,
    onDismiss: () -> Unit,
    onClearAnchor: () -> Unit,
    onAdd: (String) -> Unit,
    onReply: (parentId: String, body: String) -> Unit,
    onEdit: (commentId: String, body: String) -> Unit,
    onResolve: (commentId: String, resolved: Boolean) -> Unit,
    onDelete: (commentId: String) -> Unit,
) {
    val c = Tessera.colors
    var draft by remember { mutableStateOf("") }
    // One composer and one editor at a time: two drafts on screen is two ways to
    // lose the one being typed.
    var replyingTo by remember { mutableStateOf<String?>(null) }
    var replyDraft by remember { mutableStateOf("") }
    var editingId by remember { mutableStateOf<String?>(null) }
    var editDraft by remember { mutableStateOf("") }
    var confirmRemove by remember { mutableStateOf<DocRemoveTarget?>(null) }

    BackHandler(enabled = true) { onDismiss() }

    val focus = state.focusBlockId
    val focused = if (focus.isBlank()) emptyList() else state.groups.anchored.filter { it.blockId == focus }
    val others = if (focus.isBlank()) state.groups.anchored else state.groups.anchored.filter { it.blockId != focus }

    val rows = DocThreadRows(
        meId = meId,
        busy = state.busy,
        edit = DocDraftSlot(
            id = editingId,
            draft = editDraft,
            onDraft = { editDraft = it },
            // The body being edited is prefilled: an editor that opens empty is
            // a delete with extra steps.
            start = { id, body ->
                editingId = id
                editDraft = body
                replyingTo = null
            },
            cancel = {
                editingId = null
                editDraft = ""
            },
            submit = { id, body ->
                onEdit(id, body)
                editingId = null
                editDraft = ""
            },
        ),
        reply = DocDraftSlot(
            id = replyingTo,
            draft = replyDraft,
            onDraft = { replyDraft = it },
            start = { id, _ ->
                replyingTo = id
                replyDraft = ""
                editingId = null
            },
            cancel = {
                replyingTo = null
                replyDraft = ""
            },
            submit = { parentId, body ->
                onReply(parentId, body)
                replyingTo = null
                replyDraft = ""
            },
        ),
        onResolve = onResolve,
        onRemove = { confirmRemove = it },
    )

    Column(
        Modifier.fillMaxSize().background(c.surface).imePadding().testTag(TestTags.DOCUMENT_COMMENTS),
    ) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                stringResource(R.string.docs_comments_title),
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.padding(start = 8.dp),
            )
            if (state.openCount > 0) {
                Spacer(Modifier.width(6.dp))
                Text(
                    state.openCount.toString(),
                    color = c.text3,
                    fontSize = 13.sp,
                    modifier = Modifier.testTag(TestTags.DOCUMENT_COMMENTS_COUNT),
                )
            }
            Spacer(Modifier.weight(1f))
            IonIconButton(
                Ion.CLOSE,
                onClick = onDismiss,
                boxSize = 40.dp,
                modifier = Modifier.testTag(TestTags.DOCUMENT_COMMENTS_CLOSE),
            )
        }
        HorizontalDivider(color = c.border)

        // What a new comment will hang on. Shown because the block itself is
        // behind this panel: without the quote, «Комментарий к блоку…» is a
        // promise about a block the user can no longer see.
        if (focus.isNotBlank()) {
            Row(
                Modifier.fillMaxWidth().background(c.surfaceAlt).padding(horizontal = 12.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    state.quote.ifBlank { stringResource(R.string.docs_comments_anchor_fallback) },
                    color = c.text2,
                    fontSize = 12.sp,
                    fontStyle = FontStyle.Italic,
                    maxLines = 2,
                    modifier = Modifier.weight(1f).testTag(TestTags.DOCUMENT_COMMENTS_ANCHOR),
                )
                Spacer(Modifier.width(8.dp))
                // The tag rides the tappable box rather than the label: a
                // clickable merges its children's semantics and a tag inside
                // stops being findable (#2860).
                Box(
                    Modifier.testTag(TestTags.DOCUMENT_COMMENTS_UNPIN)
                        .clickableNoRipple(onClick = onClearAnchor)
                        .padding(4.dp),
                ) {
                    Text(
                        stringResource(R.string.docs_comments_unpin),
                        color = c.primary,
                        fontSize = 12.sp,
                        fontWeight = FontWeight.SemiBold,
                    )
                }
            }
        }

        TFormError(state.error, Modifier.padding(horizontal = 12.dp, vertical = 4.dp))

        LazyColumn(Modifier.weight(1f).fillMaxWidth()) {
            if (state.groups.isEmpty && !state.loading) {
                item {
                    Text(
                        stringResource(R.string.docs_comments_empty),
                        color = c.text3,
                        fontSize = 13.sp,
                        modifier = Modifier.fillMaxWidth().padding(24.dp)
                            .testTag(TestTags.DOCUMENT_COMMENTS_EMPTY),
                    )
                }
            }
            threadSection(R.string.docs_comments_section_focus, focused, rows)
            threadSection(R.string.docs_comments_section_blocks, others, rows)
            threadSection(R.string.docs_comments_section_document, state.groups.document, rows)
            threadSection(R.string.docs_comments_section_detached, state.groups.detached, rows)
            item { Spacer(Modifier.height(8.dp)) }
        }

        HorizontalDivider(color = c.border)
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            TTextField(
                value = draft,
                onValueChange = { draft = it },
                placeholder = stringResource(
                    if (focus.isNotBlank()) {
                        R.string.docs_comments_draft_block
                    } else {
                        R.string.docs_comments_draft_doc
                    },
                ),
                singleLine = false,
                fieldTag = TestTags.DOCUMENT_COMMENT_DRAFT,
                modifier = Modifier.weight(1f),
            )
            Spacer(Modifier.width(8.dp))
            TButton(
                stringResource(R.string.docs_comments_send),
                onClick = {
                    onAdd(draft)
                    draft = ""
                },
                enabled = draft.isNotBlank() && !state.busy,
                modifier = Modifier.testTag(TestTags.DOCUMENT_COMMENT_SEND),
            )
        }
    }

    confirmRemove?.let { target ->
        TConfirmDialog(
            title = stringResource(
                if (target.replies > 0) {
                    R.string.docs_comments_remove_thread_title
                } else {
                    R.string.docs_comments_remove_title
                },
            ),
            message = if (target.replies > 0) {
                stringResource(R.string.docs_comments_remove_thread_confirm, target.replies)
            } else {
                stringResource(R.string.docs_comments_remove_confirm)
            },
            confirmTag = TestTags.DOCUMENT_COMMENT_REMOVE_CONFIRM,
            onConfirm = {
                onDelete(target.id)
                confirmRemove = null
            },
            onDismiss = { confirmRemove = null },
        )
    }
}

/**
 * Everything a thread row needs that is not the thread itself: which row is
 * being edited, which is being answered, and what to call when one of its
 * buttons is pressed.
 *
 * Bundled rather than passed one by one because the rows are built inside a
 * [LazyListScope] — the alternative is fifteen parameters repeated at four call
 * sites, which is where a section quietly gets wired to the wrong callback.
 */
private class DocThreadRows(
    val meId: String?,
    val busy: Boolean,
    /** Editing one's own comment, and answering a thread — the same four moves
     *  (which row, its draft, start, finish) told apart by what they write. */
    val edit: DocDraftSlot,
    val reply: DocDraftSlot,
    val onResolve: (String, Boolean) -> Unit,
    val onRemove: (DocRemoveTarget) -> Unit,
)

/** One open composer: the row it belongs to, its text, and the three things that
 *  can happen to it. [submit] takes the row id so the caller does not have to
 *  read [id] back out and risk sending the draft to whichever row it changed to. */
private class DocDraftSlot(
    val id: String?,
    val draft: String,
    val onDraft: (String) -> Unit,
    val start: (String, String) -> Unit,
    val cancel: () -> Unit,
    val submit: (String, String) -> Unit,
)

private fun LazyListScope.threadSection(titleRes: Int, threads: List<DocThread>, rows: DocThreadRows) {
    if (threads.isEmpty()) return
    item(key = "section:$titleRes") {
        Text(
            stringResource(titleRes),
            color = Tessera.colors.text3,
            fontSize = 11.sp,
            fontWeight = FontWeight.SemiBold,
            modifier = Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, top = 14.dp, bottom = 2.dp),
        )
    }
    items(threads, key = { it.id }) { thread -> DocThreadCard(thread, rows) }
}

@Composable
private fun DocThreadCard(thread: DocThread, rows: DocThreadRows) {
    val c = Tessera.colors
    Column(
        Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)
            .testTag(TestTags.documentCommentThread(thread.id)),
    ) {
        // The quote is the thread's memory of the text: the block goes on being
        // edited, and without it a settled remark loses what it was about.
        if (thread.root.quote.isNotBlank()) {
            Text(
                thread.root.quote,
                color = c.text3,
                fontSize = 12.sp,
                fontStyle = FontStyle.Italic,
                maxLines = 3,
                modifier = Modifier.fillMaxWidth().quoteRail(c.border).padding(bottom = 6.dp),
            )
        }
        DocCommentRow(thread.root, thread.replies.size, rows)
        thread.replies.forEach { reply ->
            Box(Modifier.replyRail(c.border)) { DocCommentRow(reply, 0, rows) }
        }

        Row(verticalAlignment = Alignment.CenterVertically) {
            if (!thread.resolved) {
                DocCommentLink(
                    stringResource(R.string.docs_comments_reply),
                    TestTags.documentCommentReply(thread.id),
                ) { rows.reply.start(thread.id, "") }
                DocCommentLink(
                    stringResource(R.string.docs_comments_resolve),
                    TestTags.documentCommentResolve(thread.id),
                ) { rows.onResolve(thread.id, true) }
            } else {
                DocCommentLink(
                    stringResource(R.string.docs_comments_reopen),
                    TestTags.documentCommentResolve(thread.id),
                ) { rows.onResolve(thread.id, false) }
            }
        }

        if (rows.reply.id == thread.id) {
            Spacer(Modifier.height(6.dp))
            DocCommentComposer(
                value = rows.reply.draft,
                onValueChange = rows.reply.onDraft,
                placeholder = stringResource(R.string.docs_comments_reply_placeholder),
                busy = rows.busy,
                fieldTag = TestTags.DOCUMENT_COMMENT_REPLY_DRAFT,
                sendTag = TestTags.DOCUMENT_COMMENT_REPLY_SEND,
                onSend = { rows.reply.submit(thread.id, it) },
                onCancel = rows.reply.cancel,
            )
        }
        HorizontalDivider(color = c.border, modifier = Modifier.padding(top = 10.dp))
    }
}

/** One comment — header, then the body or the editor for it. [replies] is what a
 *  delete confirmation is worded with, and is zero for a reply. */
@Composable
private fun DocCommentRow(comment: DocumentComment, replies: Int, rows: DocThreadRows) {
    val c = Tessera.colors
    val res = LocalResources.current
    val own = comment.authorId != null && comment.authorId == rows.meId
    val editing = rows.edit.id == comment.id

    Column(Modifier.fillMaxWidth().padding(vertical = 4.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                docCommentAuthor(comment, stringResource(R.string.docs_comments_someone)),
                color = c.text1,
                fontSize = 13.sp,
                fontWeight = FontWeight.SemiBold,
            )
            Spacer(Modifier.width(6.dp))
            Text(whenLabel(res, comment.createdAt, LocalDateFormat.current), color = c.text3, fontSize = 11.sp)
            if (comment.isResolved) {
                Spacer(Modifier.width(6.dp))
                Text(
                    stringResource(R.string.docs_comments_resolved),
                    color = c.text3,
                    fontSize = 11.sp,
                    modifier = Modifier.testTag(TestTags.documentCommentResolved(comment.id)),
                )
            }
            if (own) {
                Spacer(Modifier.weight(1f))
                IonIconButton(
                    Ion.PENCIL,
                    { rows.edit.start(comment.id, comment.body) },
                    boxSize = 26.dp,
                    iconSize = 14.dp,
                    tint = c.text3,
                    modifier = Modifier.testTag(TestTags.documentCommentEdit(comment.id)),
                )
                IonIconButton(
                    Ion.CLOSE,
                    { rows.onRemove(DocRemoveTarget(comment.id, replies)) },
                    boxSize = 26.dp,
                    iconSize = 14.dp,
                    tint = c.text3,
                    modifier = Modifier.testTag(TestTags.documentCommentRemove(comment.id)),
                )
            }
        }
        Spacer(Modifier.height(2.dp))
        if (editing) {
            DocCommentComposer(
                value = rows.edit.draft,
                onValueChange = rows.edit.onDraft,
                placeholder = stringResource(R.string.docs_comments_edit_placeholder),
                busy = rows.busy,
                fieldTag = TestTags.DOCUMENT_COMMENT_EDIT_DRAFT,
                sendTag = TestTags.DOCUMENT_COMMENT_EDIT_SAVE,
                sendLabel = stringResource(R.string.common_save),
                onSend = { rows.edit.submit(comment.id, it) },
                onCancel = rows.edit.cancel,
            )
        } else {
            // Plain text, as on the web: a document comment is a remark about a
            // paragraph, and the web panel renders it verbatim too — markdown
            // here and not there would be a difference nobody asked for.
            Text(comment.body, color = c.text2, fontSize = 13.sp)
        }
    }
}

/** Field plus Send/Cancel — the shape both the reply and the edit take. */
@Composable
private fun DocCommentComposer(
    value: String,
    onValueChange: (String) -> Unit,
    placeholder: String,
    busy: Boolean,
    fieldTag: String,
    sendTag: String,
    onSend: (String) -> Unit,
    onCancel: () -> Unit,
    sendLabel: String = stringResource(R.string.docs_comments_send),
) {
    Column(Modifier.fillMaxWidth()) {
        TTextField(
            value = value,
            onValueChange = onValueChange,
            placeholder = placeholder,
            singleLine = false,
            fieldTag = fieldTag,
            modifier = Modifier.fillMaxWidth(),
        )
        Spacer(Modifier.height(6.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            TButton(
                sendLabel,
                onClick = { onSend(value) },
                enabled = value.isNotBlank() && !busy,
                modifier = Modifier.height(34.dp).testTag(sendTag),
            )
            Spacer(Modifier.width(6.dp))
            TButton(
                stringResource(R.string.common_cancel),
                kind = TButtonKind.Secondary,
                onClick = onCancel,
                modifier = Modifier.height(34.dp),
            )
        }
    }
}

/** A flat text link under a thread (Ответить, Решено). */
@Composable
private fun DocCommentLink(text: String, tag: String, onClick: () -> Unit) {
    Text(
        text,
        color = Tessera.colors.text3,
        fontSize = 12.sp,
        modifier = Modifier.testTag(tag).clickableNoRipple(onClick = onClick)
            .padding(top = 2.dp, end = 14.dp, bottom = 2.dp),
    )
}

/** The bar that marks quoted text, and the rail that ties replies to their root.
 *  Both are flat neutral grey: the accent gradient belongs to non-neutral
 *  elements only, and neither of these is one. */
private fun Modifier.quoteRail(color: Color): Modifier = this
    .drawBehind { drawRect(color, size = Size(2.dp.toPx(), size.height)) }
    .padding(start = 10.dp)

private fun Modifier.replyRail(color: Color): Modifier = this
    .padding(start = 6.dp)
    .drawBehind { drawRect(color, size = Size(2.dp.toPx(), size.height)) }
    .padding(start = 12.dp)
