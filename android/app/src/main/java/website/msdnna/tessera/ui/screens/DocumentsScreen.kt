package website.msdnna.tessera.ui.screens

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import kotlinx.coroutines.launch
import website.msdnna.tessera.R
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.resolve
import website.msdnna.tessera.ui.screens.documents.DocBlockView
import website.msdnna.tessera.ui.screens.documents.DocCommentsButton
import website.msdnna.tessera.ui.screens.documents.DocCommentsSheet
import website.msdnna.tessera.ui.screens.documents.DocDraft
import website.msdnna.tessera.ui.screens.documents.DocTocPanel
import website.msdnna.tessera.ui.screens.documents.DocumentActionsMenu
import website.msdnna.tessera.ui.screens.documents.DocumentComposer
import website.msdnna.tessera.ui.screens.documents.DocumentEditor
import website.msdnna.tessera.ui.screens.documents.DocumentsList
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.DocumentsViewModel
import website.msdnna.tessera.util.DocBlock
import website.msdnna.tessera.util.DocPage
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.docBlockIndex
import website.msdnna.tessera.util.docChildCount
import website.msdnna.tessera.util.docOutline
import website.msdnna.tessera.util.docSectionPages
import website.msdnna.tessera.util.docSideInsetDp

/**
 * Documents module (web `DocumentsView`) — see #2735, #2894. A grid of one
 * nesting level; tapping a tile slides a reader over it, the same master/detail
 * shape [NotesScreen] uses. Creating, renaming, nesting and deleting live here
 * too; writing the *body* happens in [DocumentEditor], the web editor embedded
 * in a WebView (§4 of #2894) — see the note there for why it is not native.
 */
@Composable
fun DocumentsScreen(workspaceId: String) {
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

    LaunchedEffect(workspaceId) {
        if (workspaceId.isNotBlank()) vm.load(workspaceId)
    }

    // The reader is an inline overlay, not a Dialog, so Back would otherwise
    // fall through to the nav back-stack. Inside a container Back walks the
    // trail out one level instead of leaving the section. The comments sheet
    // handles Back itself — closing it must not also close the reader beneath.
    BackHandler(enabled = state.openId != null && !editing && !state.comments.sheetOpen) { vm.close() }
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
                onBack = { vm.close() },
                onDraft = { draft = it },
                onChildren = { state.open?.let(vm::drillInto) },
                onEdit = { editing = true },
                onComments = { vm.openComments() },
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
                onComments = { target -> vm.openComments(target) },
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

@Composable
private fun DocumentReader(
    title: String,
    icon: String,
    blocks: List<DocBlock>,
    page: DocPage,
    loading: Boolean,
    childCount: Int,
    commentCount: Int,
    onBack: () -> Unit,
    onDraft: (DocDraft) -> Unit,
    onChildren: () -> Unit,
    onEdit: () -> Unit,
    onComments: () -> Unit,
) {
    val c = Tessera.colors
    var menuOpen by remember { mutableStateOf(false) }
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
                    onRemove = {
                        menuOpen = false
                        onDraft(DocDraft.Remove(childCount))
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
