package website.msdnna.tessera.ui.screens

import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import website.msdnna.tessera.R
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.DocEditorController
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.resolve
import website.msdnna.tessera.ui.screens.documents.DocBlockView
import website.msdnna.tessera.ui.screens.documents.DocCommentsButton
import website.msdnna.tessera.ui.screens.documents.DocCommentsSheet
import website.msdnna.tessera.ui.screens.documents.DocDraft
import website.msdnna.tessera.ui.screens.documents.DocExportMenu
import website.msdnna.tessera.ui.screens.documents.DocHistoryButton
import website.msdnna.tessera.ui.screens.documents.DocHistorySheet
import website.msdnna.tessera.ui.screens.documents.DocLinksButton
import website.msdnna.tessera.ui.screens.documents.DocLinksSheet
import website.msdnna.tessera.ui.screens.documents.DocTemplatesSheet
import website.msdnna.tessera.ui.screens.documents.DocTocPanel
import website.msdnna.tessera.ui.screens.documents.DocumentActionsMenu
import website.msdnna.tessera.ui.screens.documents.DocumentComposer
import website.msdnna.tessera.ui.screens.documents.DocumentEditor
import website.msdnna.tessera.ui.screens.documents.DocumentsList
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.DocumentsViewModel
import website.msdnna.tessera.util.DOC_IMPORT_MIME_TYPES
import website.msdnna.tessera.util.DocBlock
import website.msdnna.tessera.util.DocBuiltinTemplate
import website.msdnna.tessera.util.DocPage
import website.msdnna.tessera.util.DocTemplateCard
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.builtinTemplateCard
import website.msdnna.tessera.util.docBlockIndex
import website.msdnna.tessera.util.docChildCount
import website.msdnna.tessera.util.docExportFileName
import website.msdnna.tessera.util.docOutline
import website.msdnna.tessera.util.docSectionPages
import website.msdnna.tessera.util.docSideInsetDp
import website.msdnna.tessera.util.docTemplateGallery
import website.msdnna.tessera.util.markdownToDocJson

/**
 * Documents module (web `DocumentsView`) — see #2735, #2894. A grid of one
 * nesting level; tapping a tile slides a reader over it, the same master/detail
 * shape [NotesScreen] uses. Creating, renaming, nesting and deleting live here
 * too; writing the *body* happens in [DocumentEditor], the web editor embedded
 * in a WebView (§4 of #2894) — see the note there for why it is not native.
 */
@Composable
fun DocumentsScreen(
    workspaceId: String,
    /** A document to open straight away — the task modal's «Документы» tab
     *  navigating to the other end of a link (§7). */
    preselectDocumentId: String? = null,
    onPreselectConsumed: () -> Unit = {},
    /** Opens a linked task. Null keeps the links panel read-only, which is what
     *  a host with nowhere to navigate to should get. */
    onOpenTask: ((String) -> Unit)? = null,
) {
    val c = Tessera.colors
    val vm: DocumentsViewModel = viewModel()
    val state by vm.state.collectAsStateWithLifecycle()
    // Who «me» is — only one's own comments offer edit and delete (§5).
    val me by AppContainer.prefs.user.collectAsStateWithLifecycle(initialValue = null)
    var draft by remember { mutableStateOf<DocDraft?>(null) }
    // The editor is a surface over the reader, not a route: the reader is where
    // it came from and where closing it lands, and the back stack has no idea
    // this section has two layers.
    var editing by remember { mutableStateOf(false) }
    // Hoisted out of the editor: a rollback taken from the journal (§6) has to
    // reach the surface that is holding the pre-rollback text, and the journal
    // is a sibling of the editor rather than a child of it.
    val editorController = remember { DocEditorController() }
    val ctx = LocalContext.current
    // Both are read in the composition and used outside it: the share sheet is
    // shown from a callback, where `ctx.getString` would give the system's
    // language rather than the one set in the profile.
    val res = LocalResources.current
    val exportFallbackName = stringResource(R.string.docs_export_name)
    val importFallbackTitle = stringResource(R.string.docs_import_title)
    val builtinBodies = builtinTemplateBodies()
    val scope = rememberCoroutineScope()

    // One picker for every import: which road a file takes is decided from its
    // name (docImportRoute), and a stricter MIME filter would grey out files the
    // server would have accepted.
    val filePicker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        uri ?: return@rememberLauncherForActivityResult
        scope.launch {
            val bytes = withContext(Dispatchers.IO) {
                ctx.contentResolver.openInputStream(uri)?.use { it.readBytes() }
            }
            if (bytes == null || bytes.isEmpty()) return@launch
            vm.importFile(
                fileName = pickedFileName(ctx, uri),
                bytes = bytes,
                mime = ctx.contentResolver.getType(uri),
                fallbackTitle = importFallbackTitle,
            )
        }
    }

    LaunchedEffect(workspaceId) {
        if (workspaceId.isNotBlank()) {
            vm.load(workspaceId)
            // Asked once, with the list: it decides which formats the export
            // menu offers and which files the picker will take, and both of
            // those are needed before the user reaches for them.
            vm.loadConverter()
        }
    }

    // A converted file is waiting for the editor to parse it (§8). Opening the
    // editor is the *only* way that body gets written, so it is opened here
    // rather than offered — the alternative is a document the user watched
    // appear and then found empty.
    LaunchedEffect(state.pendingImport, state.openId) {
        if (state.pendingImport != null && state.openId == state.pendingImport?.documentId) {
            editing = true
        }
    }

    // Opened from a task's «Документы» tab. Waits for the list: the reader is
    // handed a Document, and the id alone is a tile that does not exist yet.
    // A document that is not in the list (deleted, or in another workspace) is
    // consumed all the same — a preselect that never clears would reopen on
    // every visit to the section.
    LaunchedEffect(preselectDocumentId, state.docs) {
        val wanted = preselectDocumentId ?: return@LaunchedEffect
        if (state.docs.isEmpty()) return@LaunchedEffect
        state.docs.firstOrNull { it.id == wanted }?.let(vm::open)
        onPreselectConsumed()
    }

    // The reader is an inline overlay, not a Dialog, so Back would otherwise
    // fall through to the nav back-stack. Inside a container Back walks the
    // trail out one level instead of leaving the section. The comments sheet
    // handles Back itself — closing it must not also close the reader beneath.
    BackHandler(
        enabled = state.openId != null && !editing && !state.comments.sheetOpen &&
            !state.history.sheetOpen && !state.links.sheetOpen,
    ) { vm.close() }
    BackHandler(enabled = state.openId == null && state.trail.isNotEmpty()) {
        vm.crumbTo(state.trail.lastIndex - 1)
    }

    Box(Modifier.fillMaxSize().background(c.bg).testTag(TestTags.DOCUMENTS_SCREEN)) {
        if (state.loading) {
            Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { TesseraLoader() }
        } else {
            DocumentsList(
                tiles = state.tiles,
                trail = state.trail,
                childCount = { id -> docChildCount(state.docs, id) },
                busy = state.busy,
                onCrumb = vm::crumbTo,
                onOpen = vm::open,
                onCreate = { draft = DocDraft.Create },
                onTemplates = { vm.openTemplates() },
            )
        }

        state.error?.let { message ->
            Box(Modifier.fillMaxSize().padding(16.dp), contentAlignment = Alignment.BottomCenter) {
                Text(message.resolve(), color = c.text3, fontSize = 12.sp)
            }
        }

        if (state.openId != null) {
            DocumentReader(
                title = state.open?.title.orEmpty(),
                icon = state.open?.icon.orEmpty(),
                blocks = state.blocks,
                page = state.page,
                loading = state.opening,
                childCount = state.openChildCount,
                commentCount = state.comments.openCount,
                linkCount = state.links.linkCount,
                onBack = { vm.close() },
                onDraft = { draft = it },
                onChildren = { state.open?.let(vm::drillInto) },
                onEdit = { editing = true },
                onComments = { vm.openComments() },
                onHistory = { vm.openHistory() },
                onLinks = { vm.openLinks() },
                exportFormats = state.templates.exportFormats,
                onExport = { format ->
                    val title = state.open?.title.orEmpty()
                    vm.exportOpen(
                        cacheDir = ctx.cacheDir,
                        format = format,
                        fileName = docExportFileName(title, format, exportFallbackName),
                    ) { file -> shareExportedFile(ctx, file, res.getString(R.string.docs_export_share, file.name)) }
                },
            )
        }

        // A document with no slug cannot be addressed by the embed URL. That is
        // not a state the API produces (the slug is assigned on create), but a
        // reader over a half-loaded tile has no document at all yet — and an
        // editor opened on nothing would be a blank white screen with a caret.
        val open = state.open
        if (editing && open != null && open.slug.isNotBlank()) {
            DocumentEditor(
                title = open.title,
                slug = open.slug,
                workspaceId = workspaceId,
                serverRoot = RetrofitClient.serverRoot,
                commentCount = state.comments.openCount,
                linkCount = state.links.linkCount,
                controller = editorController,
                // Handed over once the page says it is up: the bridge is
                // installed on mount, and calling into it before that is a
                // no-op that would lose the import silently.
                pendingImport = state.pendingImport?.takeIf { it.documentId == open.id }?.payload,
                onImportApplied = { vm.clearPendingImport() },
                onComments = { target -> vm.openComments(target) },
                onHistory = { vm.openHistory() },
                onLinks = { target -> vm.openLinks(target) },
                onClose = {
                    editing = false
                    // The reader under it is showing the text from before the edit.
                    vm.refreshOpen()
                },
            )
        }

        // Above the editor on purpose: a discussion is opened *from* it, and the
        // page underneath must not be the thing that takes the tap. Closing the
        // sheet lands back on whichever of the two was showing.
        if (state.comments.sheetOpen) {
            DocCommentsSheet(
                state = state.comments,
                meId = me?.id,
                onDismiss = { vm.closeComments() },
                onClearAnchor = { vm.clearCommentAnchor() },
                onAdd = { body -> vm.addComment(body) },
                onReply = { parentId, body -> vm.replyComment(parentId, body) },
                onEdit = { id, body -> vm.editComment(id, body) },
                onResolve = { id, resolved -> vm.resolveComment(id, resolved) },
                onDelete = { id -> vm.deleteComment(id) },
            )
        }

        if (state.links.sheetOpen) {
            DocLinksSheet(
                state = state.links,
                meId = me?.id,
                onDismiss = { vm.closeLinks() },
                onClearAnchor = { vm.clearLinkAnchor() },
                onLink = { taskId -> vm.linkTask(taskId) },
                onUnlink = { linkId -> vm.unlinkTask(linkId) },
                onRaise = { title, mode, approvers -> vm.raiseApproval(title, mode, approvers) },
                onDecide = { id, decision, comment -> vm.decideApproval(id, decision, comment) },
                onCancel = { id -> vm.cancelApproval(id) },
                onOpenTask = onOpenTask?.let { open -> { link -> open(link.taskId) } },
            )
        }

        if (state.history.sheetOpen) {
            DocHistorySheet(
                state = state.history,
                onDismiss = { vm.closeHistory() },
                onSelect = { id -> vm.selectVersion(id) },
                onSnapshot = { label -> vm.snapshotVersion(label) },
                onRestore = { id -> vm.restoreVersion(id) },
            )
        }

        // A rollback replaced the text under an open editor. Reloading it is not
        // politeness — the editor is still holding the pre-rollback version, and
        // its next autosave would answer the rollback with a conflict.
        LaunchedEffect(state.history.restoreTick) {
            if (state.history.restoreTick > 0 && editing) editorController.reload()
        }

        if (state.templates.sheetOpen) {
            DocTemplatesSheet(
                cards = docTemplateGallery(state.templates.saved, builtinTemplateCards()),
                loading = state.templates.loading,
                importing = state.templates.importing,
                busy = state.templates.busy,
                error = state.templates.error,
                converterAvailable = state.templates.converterAvailable,
                converterReason = state.templates.converterReason,
                onDismiss = { vm.closeTemplates() },
                // A saved template is applied server-side (its body lives there);
                // a built-in has no server side at all, so its Markdown is
                // parsed here and written through the ordinary content endpoint.
                onUse = { card ->
                    if (card.builtin) {
                        vm.useTemplate(
                            cardId = card.id,
                            title = card.title,
                            templateId = null,
                            content = markdownToDocJson(builtinBodies[card.builtinKey].orEmpty()),
                        )
                    } else {
                        vm.useTemplate(card.id, card.title, card.id, null)
                    }
                },
                onRemove = { card -> vm.deleteTemplate(card.id) },
                onPickFile = { filePicker.launch(DOC_IMPORT_MIME_TYPES) },
            )
        }

        // What the conversion could not carry over. A line rather than a toast:
        // it is the only place the reason appears, and a document that is a
        // reduction of its file should say so until it is read.
        state.templates.notice?.let { notice ->
            Box(Modifier.fillMaxSize().padding(16.dp), contentAlignment = Alignment.BottomCenter) {
                Text(
                    notice.resolve(),
                    color = c.text2,
                    fontSize = 12.sp,
                    modifier = Modifier
                        .background(c.surfaceAlt)
                        .clickable { vm.clearTemplatesNotice() }
                        .padding(horizontal = 12.dp, vertical = 8.dp)
                        .testTag(TestTags.DOCUMENT_IMPORT_NOTICE),
                )
            }
        }

        DocumentComposer(
            draft = draft,
            onDismiss = { draft = null },
            onCreate = { title ->
                draft = null
                vm.create(title)
            },
            onNested = { title ->
                draft = null
                vm.createNested(title)
            },
            onRename = { title ->
                draft = null
                vm.rename(title)
            },
            onRemove = {
                draft = null
                vm.remove()
            },
        )
    }
}

/**
 * The built-in starters as gallery cards, with their text taken from the
 * resources — an English document must not open with Russian headings (#2799).
 */
@Composable
private fun builtinTemplateCards(): List<DocTemplateCard> = DocBuiltinTemplate.entries.map { tpl ->
    builtinTemplateCard(
        template = tpl,
        title = stringResource(builtinTitle(tpl)),
        description = stringResource(builtinDescription(tpl)),
    )
}

/** Their Markdown bodies, keyed the same way the cards are. */
@Composable
private fun builtinTemplateBodies(): Map<String, String> =
    DocBuiltinTemplate.entries.associate { it.key to stringResource(builtinBody(it)) }

private fun builtinTitle(tpl: DocBuiltinTemplate): Int = when (tpl) {
    DocBuiltinTemplate.MEETING -> R.string.docs_template_meeting_title
    DocBuiltinTemplate.SPEC -> R.string.docs_template_spec_title
    DocBuiltinTemplate.RETRO -> R.string.docs_template_retro_title
}

private fun builtinDescription(tpl: DocBuiltinTemplate): Int = when (tpl) {
    DocBuiltinTemplate.MEETING -> R.string.docs_template_meeting_description
    DocBuiltinTemplate.SPEC -> R.string.docs_template_spec_description
    DocBuiltinTemplate.RETRO -> R.string.docs_template_retro_description
}

private fun builtinBody(tpl: DocBuiltinTemplate): Int = when (tpl) {
    DocBuiltinTemplate.MEETING -> R.string.docs_template_meeting_body
    DocBuiltinTemplate.SPEC -> R.string.docs_template_spec_body
    DocBuiltinTemplate.RETRO -> R.string.docs_template_retro_body
}

/**
 * Hands an exported file to the system share sheet.
 *
 * [chooserTitle] is resolved in the composition: the dialog is shown from
 * outside it, where `ctx.getString` would answer in the system's language
 * rather than the one set in the profile.
 */
private fun shareExportedFile(ctx: android.content.Context, file: java.io.File, chooserTitle: String) {
    val uri = androidx.core.content.FileProvider.getUriForFile(
        ctx,
        "${ctx.packageName}.fileprovider",
        file,
    )
    val send = android.content.Intent(android.content.Intent.ACTION_SEND).apply {
        type = ctx.contentResolver.getType(uri) ?: "application/octet-stream"
        putExtra(android.content.Intent.EXTRA_STREAM, uri)
        addFlags(
            android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION or
                android.content.Intent.FLAG_ACTIVITY_NEW_TASK,
        )
    }
    val chooser = android.content.Intent.createChooser(send, chooserTitle)
        .apply { addFlags(android.content.Intent.FLAG_ACTIVITY_NEW_TASK) }
    runCatching { ctx.startActivity(chooser) }
}

/** The picked file's display name — the whole of what decides its import road. */
private fun pickedFileName(ctx: android.content.Context, uri: android.net.Uri): String = runCatching {
    ctx.contentResolver.query(uri, null, null, null, null)?.use { cursor ->
        val idx = cursor.getColumnIndex(android.provider.OpenableColumns.DISPLAY_NAME)
        if (idx >= 0 && cursor.moveToFirst()) cursor.getString(idx) else null
    }
}.getOrNull() ?: uri.lastPathSegment.orEmpty()

@Composable
private fun DocumentReader(
    title: String,
    icon: String,
    blocks: List<DocBlock>,
    page: DocPage,
    loading: Boolean,
    childCount: Int,
    commentCount: Int,
    linkCount: Int,
    exportFormats: List<String>,
    onBack: () -> Unit,
    onDraft: (DocDraft) -> Unit,
    onChildren: () -> Unit,
    onEdit: () -> Unit,
    onComments: () -> Unit,
    onHistory: () -> Unit,
    onLinks: () -> Unit,
    onExport: (String) -> Unit,
) {
    val c = Tessera.colors
    var menuOpen by remember { mutableStateOf(false) }
    var exportOpen by remember { mutableStateOf(false) }
    var tocOpen by remember { mutableStateOf(false) }
    val listState = rememberLazyListState()
    val scope = rememberCoroutineScope()
    val outline = remember(blocks) { docOutline(blocks) }
    // One geometry per block: a section break switches it for everything after
    // it, and a document without one has the single geometry it always had.
    val pages = remember(blocks, page) { docSectionPages(blocks, page) }

    Column(Modifier.fillMaxSize().background(c.surface).testTag(TestTags.DOCUMENT_READER)) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IonIconButton(
                Ion.CHEVRON_FORWARD,
                onClick = onBack,
                boxSize = 40.dp,
                modifier = Modifier.graphicsLayer { scaleX = -1f }.testTag(TestTags.DOCUMENT_BACK),
            )
            Spacer(Modifier.width(4.dp))
            if (icon.isNotBlank()) {
                Text(icon, fontSize = 16.sp)
                Spacer(Modifier.width(6.dp))
            }
            Text(
                title.ifBlank { stringResource(R.string.docs_reader_untitled) },
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                modifier = Modifier.weight(1f),
            )
            IonIconButton(
                Ion.PENCIL,
                onClick = onEdit,
                boxSize = 40.dp,
                modifier = Modifier.testTag(TestTags.DOCUMENT_EDIT),
            )
            DocCommentsButton(count = commentCount, onClick = onComments)
            DocLinksButton(count = linkCount, onClick = onLinks)
            DocHistoryButton(onClick = onHistory)
            IonIconButton(
                Ion.LIST,
                onClick = { tocOpen = true },
                boxSize = 40.dp,
                modifier = Modifier.testTag(TestTags.DOCUMENT_TOC_OPEN),
            )
            // The menu is a sibling of its trigger inside this Box: TDropdown
            // positions itself against the anchor's bounds.
            Box {
                IonIconButton(
                    Ion.ELLIPSIS_V,
                    onClick = { menuOpen = true },
                    boxSize = 40.dp,
                    modifier = Modifier.testTag(TestTags.DOCUMENT_ACTIONS),
                )
                DocumentActionsMenu(
                    expanded = menuOpen,
                    childCount = childCount,
                    onDismiss = { menuOpen = false },
                    onNested = {
                        menuOpen = false
                        onDraft(DocDraft.Nested)
                    },
                    onChildren = {
                        menuOpen = false
                        onChildren()
                    },
                    onRename = {
                        menuOpen = false
                        onDraft(DocDraft.Rename(title))
                    },
                    onExport = {
                        menuOpen = false
                        exportOpen = true
                    },
                    onRemove = {
                        menuOpen = false
                        onDraft(DocDraft.Remove(childCount))
                    },
                )
                DocExportMenu(
                    expanded = exportOpen,
                    formats = exportFormats,
                    onDismiss = { exportOpen = false },
                    onPick = { format ->
                        exportOpen = false
                        onExport(format)
                    },
                )
            }
        }
        HorizontalDivider(color = c.border)

        when {
            loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { TesseraLoader() }

            blocks.isEmpty() -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text(stringResource(R.string.docs_reader_empty), color = c.text3, fontSize = 14.sp)
            }

            else -> BoxWithConstraints(Modifier.fillMaxSize()) {
                val available = maxWidth.value.toDouble()
                LazyColumn(
                    Modifier.fillMaxSize(),
                    state = listState,
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    item { Spacer(Modifier.height(8.dp)) }
                    itemsIndexed(blocks, key = { _, block -> block.id }) { index, block ->
                        // The document's own margins, as a share of its sheet —
                        // see docSideInsetDp for why a phone cannot take them in
                        // millimetres.
                        val inset = docSideInsetDp(pages[index], available).dp
                        Box(Modifier.fillMaxWidth().padding(horizontal = inset)) { DocBlockView(block) }
                    }
                    item { Spacer(Modifier.height(24.dp)) }
                }
            }
        }
    }

    if (tocOpen) {
        DocTocPanel(
            rows = outline,
            onDismiss = { tocOpen = false },
            onJump = { row ->
                tocOpen = false
                val index = docBlockIndex(blocks, row.id)
                // +1 for the leading spacer item; a heading that is no longer in
                // the body (the outline was built from an older parse) simply
                // does not move the list.
                if (index >= 0) scope.launch { listState.scrollToItem(index + 1) }
            },
        )
    }
}
