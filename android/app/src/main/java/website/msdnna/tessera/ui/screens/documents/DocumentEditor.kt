package website.msdnna.tessera.ui.screens.documents

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
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
import kotlinx.coroutines.delay
import website.msdnna.tessera.R
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.DocEditorController
import website.msdnna.tessera.ui.components.DocEditorWebView
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.DocSaveStatus
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.docEmbedUrl

/**
 * Editing surface for one document (#2894 §4): a native bar over the web
 * editor.
 *
 * The bar is where the app stays the app — back, the title, and the one thing
 * autosave cannot say for itself, namely whether the text on screen is on the
 * server. The status is *reported* by the page rather than guessed here: two
 * sides deciding separately is how «сохранено» ends up over unsaved text.
 */
@Composable
fun DocumentEditor(
    title: String,
    slug: String,
    workspaceId: String,
    serverRoot: String,
    onClose: () -> Unit,
) {
    val c = Tessera.colors
    val scope = rememberCoroutineScope()
    val controller = remember { DocEditorController() }
    var status by remember { mutableStateOf(DocSaveStatus.SAVED) }
    var failed by remember { mutableStateOf(false) }
    // Someone else saved while we were typing. Reloading under a live caret is
    // worse than being seconds stale, so it is offered, not done.
    var remoteChanged by remember { mutableStateOf(false) }
    // Who holds the block the user just tried to type in. Refusing a keystroke
    // silently reads as a broken editor, so it is said out loud — briefly, and
    // in the app's own chrome rather than as a web toast inside a native screen.
    // null is "no notice"; an empty string is a holder we have no name for
    // (a viewer who joined the room after us) — still worth saying.
    var blockedBy by remember { mutableStateOf<String?>(null) }
    val language = stringResource(R.string.docs_embed_lang)
    val url = remember(serverRoot, slug, workspaceId, c.isDark, language) {
        docEmbedUrl(
            serverRoot = serverRoot,
            slug = slug,
            workspaceId = workspaceId,
            token = RetrofitClient.authToken,
            dark = c.isDark,
            language = language,
        )
    }

    /** Leaving writes first: the debounce may still be holding the last words. */
    fun close() {
        controller.save()
        onClose()
    }

    BackHandler { close() }

    Column(
        Modifier.fillMaxSize().background(c.surface).imePadding().testTag(TestTags.DOCUMENT_EDITOR),
    ) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IonIconButton(
                Ion.CHEVRON_FORWARD,
                onClick = { close() },
                boxSize = 40.dp,
                modifier = Modifier.graphicsLayer { scaleX = -1f }.testTag(TestTags.DOCUMENT_EDITOR_CLOSE),
            )
            Spacer(Modifier.width(4.dp))
            Text(
                title.ifBlank { stringResource(R.string.docs_reader_untitled) },
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                modifier = Modifier.weight(1f),
            )
            Text(
                stringResource(statusLabel(status)),
                color = if (status == DocSaveStatus.SAVED) c.text3 else c.text2,
                fontSize = 12.sp,
                modifier = Modifier.testTag(TestTags.DOCUMENT_EDITOR_STATUS),
            )
        }
        HorizontalDivider(color = c.border)

        // A conflict stops autosave dead: the server has someone else's version
        // and retrying would overwrite it. The only way out is taking theirs,
        // which is destructive enough to be a button rather than a timer.
        if (status == DocSaveStatus.CONFLICT || remoteChanged) {
            val conflict = status == DocSaveStatus.CONFLICT
            Row(
                Modifier.fillMaxWidth().background(c.surfaceAlt).padding(horizontal = 12.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(
                    stringResource(
                        if (conflict) R.string.docs_editor_conflict else R.string.docs_editor_remote_change,
                    ),
                    color = c.text2,
                    fontSize = 12.sp,
                    modifier = Modifier.weight(1f),
                )
                // The tag rides the tappable box, not the label: a clickable
                // merges its children's semantics, and a tag inside would stop
                // being findable (the trap #2860 hit).
                Box(
                    Modifier
                        .testTag(TestTags.DOCUMENT_EDITOR_RELOAD)
                        .clickable {
                            remoteChanged = false
                            controller.reload()
                        }
                        .padding(4.dp),
                ) {
                    Text(
                        stringResource(R.string.docs_editor_reload),
                        color = c.primary,
                        fontSize = 12.sp,
                        fontWeight = FontWeight.SemiBold,
                    )
                }
            }
        }

        val holder = blockedBy
        if (holder != null) {
            LaunchedEffect(holder) {
                delay(BLOCKED_NOTICE_MS)
                if (blockedBy == holder) blockedBy = null
            }
            Text(
                stringResource(
                    R.string.docs_editor_blocked,
                    holder.ifBlank { stringResource(R.string.docs_editor_blocked_someone) },
                ),
                color = c.text2,
                fontSize = 12.sp,
                modifier = Modifier
                    .fillMaxWidth()
                    .background(c.surfaceAlt)
                    .padding(horizontal = 12.dp, vertical = 8.dp)
                    .testTag(TestTags.DOCUMENT_EDITOR_BLOCKED),
            )
        }

        if (failed) {
            Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text(
                    stringResource(R.string.docs_editor_failed),
                    color = c.text3,
                    fontSize = 13.sp,
                    modifier = Modifier.padding(24.dp).testTag(TestTags.DOCUMENT_EDITOR_FAILED),
                )
            }
        } else {
            DocEditorWebView(
                url = url,
                controller = controller,
                scope = scope,
                modifier = Modifier.fillMaxSize(),
                onStatus = { signal -> status = signal.status },
                onBlocked = { name -> blockedBy = name },
                onRemoteChange = { remoteChanged = true },
                onLoadFailed = { failed = true },
            )
        }
    }
}

/** How long the "someone holds this block" line stays up. Long enough to read,
 *  short enough not to sit over the text after the lock is gone. */
private const val BLOCKED_NOTICE_MS = 3000L

private fun statusLabel(status: DocSaveStatus): Int = when (status) {
    DocSaveStatus.SAVED -> R.string.docs_editor_saved
    DocSaveStatus.DIRTY -> R.string.docs_editor_dirty
    DocSaveStatus.SAVING -> R.string.docs_editor_saving
    DocSaveStatus.CONFLICT -> R.string.docs_editor_conflict_short
    DocSaveStatus.ERROR -> R.string.docs_editor_error
}
