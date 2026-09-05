package website.msdnna.tessera.ui.screens.documents

import android.content.res.Resources
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.DocumentVersion
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmDialog
import website.msdnna.tessera.ui.components.TFormError
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.LocalDateFormat
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.DocHistoryState
import website.msdnna.tessera.util.DateFormatPrefs
import website.msdnna.tessera.util.DocDiffRow
import website.msdnna.tessera.util.DocDiffStatus
import website.msdnna.tessera.util.DocDiffSummary
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.docDiffSummary
import website.msdnna.tessera.util.docVersionDiff
import website.msdnna.tessera.util.timeLabel
import website.msdnna.tessera.util.whenLabel

/** The way into the journal, over the reader and over the editor alike. */
@Composable
fun DocHistoryButton(onClick: () -> Unit, modifier: Modifier = Modifier) {
    IonIconButton(
        Ion.TIME,
        onClick = onClick,
        boxSize = 40.dp,
        modifier = modifier.testTag(TestTags.DOCUMENT_HISTORY_OPEN),
    )
}

/**
 * The version journal of the open document (#2731, §6 of #2894).
 *
 * A panel over the document rather than beside it, for the same reason the
 * discussions are: the web has a column to spare next to the sheet and a phone
 * has none. The consequence is that the comparison cannot sit under the list
 * *and* stay readable, so it takes the lower part of the panel only while an
 * entry is picked — walking a journal means switching entries with the diff
 * open, and a dialog per entry would make that a close-and-reopen each time.
 *
 * Restoring is the one destructive move here, so it is confirmed; what the
 * confirmation says is the thing that makes it safe — the current state is
 * snapshotted server-side before the rollback, so this is not a one-way door.
 */
@Composable
fun DocHistorySheet(
    state: DocHistoryState,
    onDismiss: () -> Unit,
    onSelect: (String) -> Unit,
    onSnapshot: (String) -> Unit,
    onRestore: (String) -> Unit,
) {
    val c = Tessera.colors
    var naming by remember { mutableStateOf(false) }
    var label by remember { mutableStateOf("") }
    var confirmRestore by remember { mutableStateOf<String?>(null) }
    val imageLabel = stringResource(R.string.docs_history_diff_image)

    BackHandler(enabled = true) { onDismiss() }

    // Recomputed only when the selection or a fetched body changes: a diff over
    // a long document walks every block, and a recomposition of the panel (a
    // keystroke in the snapshot name) must not drag it along.
    val rows = remember(state.selectedId, state.versions, state.bodies, imageLabel) {
        docVersionDiff(state.selected, state.baseline, state.bodies, imageLabel)
    }
    val summary = remember(rows) { docDiffSummary(rows) }

    Column(Modifier.fillMaxSize().background(c.surface).imePadding().testTag(TestTags.DOCUMENT_HISTORY)) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                stringResource(R.string.docs_history_title),
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.padding(start = 8.dp),
            )
            if (state.loading) {
                Spacer(Modifier.width(6.dp))
                Text("…", color = c.text3, fontSize = 13.sp)
            }
            Spacer(Modifier.weight(1f))
            IonIconButton(
                Ion.CLOSE,
                onClick = onDismiss,
                boxSize = 40.dp,
                modifier = Modifier.testTag(TestTags.DOCUMENT_HISTORY_CLOSE),
            )
        }
        HorizontalDivider(color = c.border)

        // Taking a snapshot is a two-step: the name is the whole point of a
        // manual entry (retention never prunes it, and «Версия 12» is not what
        // anyone scrolls the journal to find), but demanding one before the
        // button does anything would make the common case slower.
        Column(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp)) {
            if (!naming) {
                TButton(
                    stringResource(R.string.docs_history_snapshot),
                    kind = TButtonKind.Secondary,
                    onClick = { naming = true },
                    enabled = !state.busy,
                    modifier = Modifier.testTag(TestTags.DOCUMENT_SNAPSHOT),
                )
            } else {
                TTextField(
                    value = label,
                    onValueChange = { label = it },
                    placeholder = stringResource(R.string.docs_history_label_hint),
                    fieldTag = TestTags.DOCUMENT_SNAPSHOT_LABEL,
                    modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(6.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    TButton(
                        stringResource(R.string.common_save),
                        onClick = {
                            onSnapshot(label)
                            label = ""
                            naming = false
                        },
                        enabled = !state.busy,
                        modifier = Modifier.height(34.dp).testTag(TestTags.DOCUMENT_SNAPSHOT_SAVE),
                    )
                    TButton(
                        stringResource(R.string.common_cancel),
                        kind = TButtonKind.Secondary,
                        onClick = {
                            naming = false
                            label = ""
                        },
                        modifier = Modifier.height(34.dp),
                    )
                }
            }
        }

        TFormError(state.error, Modifier.padding(horizontal = 12.dp, vertical = 4.dp))

        LazyColumn(Modifier.weight(1f).fillMaxWidth()) {
            if (state.versions.isEmpty() && !state.loading) {
                item {
                    Text(
                        stringResource(R.string.docs_history_empty),
                        color = c.text3,
                        fontSize = 13.sp,
                        modifier = Modifier.fillMaxWidth().padding(24.dp)
                            .testTag(TestTags.DOCUMENT_HISTORY_EMPTY),
                    )
                }
            }
            // Newest first, as the server returns them: the question a journal
            // answers most often is «что изменилось с тех пор».
            items(state.versions, key = { it.id }) { version ->
                DocVersionRow(
                    version = version,
                    selected = version.id == state.selectedId,
                    onClick = { onSelect(version.id) },
                )
            }
            item { Spacer(Modifier.height(8.dp)) }
        }

        if (state.selectedId.isNotEmpty()) {
            HorizontalDivider(color = c.border)
            DocVersionDiff(
                baseline = state.baseline,
                ready = state.ready,
                rows = rows,
                summary = summary,
                busy = state.busy,
                onRestore = { confirmRestore = state.selectedId },
                modifier = Modifier.weight(DIFF_SHARE),
            )
        }
    }

    confirmRestore?.let { versionId ->
        TConfirmDialog(
            title = stringResource(R.string.docs_history_restore_title),
            message = stringResource(R.string.docs_history_restore_confirm),
            confirmTag = TestTags.DOCUMENT_RESTORE_CONFIRM,
            onConfirm = {
                onRestore(versionId)
                confirmRestore = null
            },
            onDismiss = { confirmRestore = null },
        )
    }
}

/** How much of the panel the comparison takes while an entry is picked. The list
 *  keeps the larger share: switching entries is what a journal is walked with. */
private const val DIFF_SHARE = 0.8f

/**
 * One journal entry.
 *
 * A manual snapshot reads differently from an autosaved session — it is the
 * entry people scroll the journal to find — so its revision takes the accent
 * and it carries the bookmark. The rest stays neutral, including the selected
 * one's frame: the accent gradient belongs to non-neutral elements, and a row
 * of a list is not one.
 */
@Composable
private fun DocVersionRow(version: DocumentVersion, selected: Boolean, onClick: () -> Unit) {
    val c = Tessera.colors
    val res = LocalResources.current
    val fmt = LocalDateFormat.current

    Column(
        Modifier.fillMaxWidth()
            .clickableNoRipple(onClick = onClick)
            .background(if (selected) c.surfaceAlt else Color.Transparent)
            .padding(horizontal = 16.dp, vertical = 10.dp)
            .testTag(TestTags.documentVersionRow(version.id)),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            if (version.manual) {
                IonIcon(Ion.BOOKMARK, size = 12.dp, tint = c.primary)
                Spacer(Modifier.width(4.dp))
            }
            Text(
                stringResource(R.string.docs_history_revision, version.revision),
                color = if (version.manual) c.primary else c.text2,
                fontSize = 11.sp,
                fontWeight = FontWeight.SemiBold,
            )
            Spacer(Modifier.width(6.dp))
            Text(versionSpan(res, version, fmt), color = c.text3, fontSize = 11.sp)
        }
        if (version.label.isNotBlank()) {
            Text(version.label, color = c.text1, fontSize = 12.sp)
        }
        Text(versionAuthor(version, stringResource(R.string.docs_history_unknown_author)), color = c.text3, fontSize = 11.sp)
        Text(
            version.preview.ifBlank { stringResource(R.string.docs_empty_preview) },
            color = c.text3,
            fontSize = 11.sp,
            maxLines = 2,
            modifier = Modifier.padding(top = 2.dp),
        )
    }
}

/**
 * An entry covers an editing session, so it has a span rather than a moment.
 * A same-minute end is not shown — «13:05–13:05» is noise.
 */
private fun versionSpan(
    res: Resources,
    version: DocumentVersion,
    fmt: DateFormatPrefs,
): String {
    val from = whenLabel(res, version.createdAt, fmt)
    val to = timeLabel(res, version.updatedAt, fmt)
    if (to.isEmpty() || from.endsWith(to)) return from
    return res.getString(R.string.docs_history_span, from, to)
}

/** The author, with the fallback chain the discussion cards use — the same
 *  person must not be «Иван» in one panel and an email address in the other. */
private fun versionAuthor(version: DocumentVersion, fallback: String): String {
    val name = version.authorName?.trim().orEmpty()
    if (name.isNotEmpty()) return name
    val local = version.authorEmail.orEmpty().substringBefore('@').trim()
    return local.ifEmpty { fallback }
}

/** The comparison of the picked entry against the newest one. */
@Composable
private fun DocVersionDiff(
    baseline: DocumentVersion?,
    ready: Boolean,
    rows: List<DocDiffRow>,
    summary: DocDiffSummary,
    busy: Boolean,
    onRestore: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val c = Tessera.colors

    Column(modifier.fillMaxWidth().testTag(TestTags.DOCUMENT_HISTORY_DIFF)) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                stringResource(R.string.docs_history_compare, baseline?.revision ?: 0),
                color = c.text3,
                fontSize = 11.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.weight(1f),
            )
            TButton(
                stringResource(R.string.docs_history_restore),
                kind = TButtonKind.Secondary,
                onClick = onRestore,
                enabled = !busy,
                modifier = Modifier.height(32.dp).testTag(TestTags.DOCUMENT_RESTORE),
            )
        }

        when {
            !ready -> DiffNotice(stringResource(R.string.docs_history_loading_version))

            summary.identical -> DiffNotice(stringResource(R.string.docs_history_identical))

            else -> {
                Row(
                    Modifier.fillMaxWidth().padding(horizontal = 16.dp).testTag(TestTags.DOCUMENT_HISTORY_SUMMARY),
                    horizontalArrangement = Arrangement.spacedBy(10.dp),
                ) {
                    if (summary.added > 0) {
                        DiffCount(pluralStringResource(R.plurals.docs_history_added, summary.added, summary.added))
                    }
                    if (summary.removed > 0) {
                        DiffCount(pluralStringResource(R.plurals.docs_history_removed, summary.removed, summary.removed))
                    }
                    if (summary.changed > 0) {
                        DiffCount(stringResource(R.string.docs_history_changed, summary.changed))
                    }
                    if (summary.moved > 0) {
                        DiffCount(stringResource(R.string.docs_history_moved, summary.moved))
                    }
                }
                LazyColumn(Modifier.fillMaxWidth().padding(top = 6.dp)) {
                    items(rows.size) { index -> DocDiffRowView(rows[index]) }
                    item { Spacer(Modifier.height(8.dp)) }
                }
            }
        }
    }
}

@Composable
private fun DiffNotice(text: String) {
    Text(
        text,
        color = Tessera.colors.text3,
        fontSize = 12.sp,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp),
    )
}

@Composable
private fun DiffCount(text: String) {
    Text(text, color = Tessera.colors.text3, fontSize = 11.sp)
}

/**
 * One block of the comparison.
 *
 * Status is carried by a left rail *and* a word, never by colour alone: the two
 * colours that read as «добавлено» and «удалено» are also the two most common
 * forms of colour blindness.
 */
@Composable
private fun DocDiffRowView(row: DocDiffRow) {
    val c = Tessera.colors
    val rail = when (row.status) {
        DocDiffStatus.ADDED -> DIFF_ADDED_COLOR
        DocDiffStatus.REMOVED -> DIFF_REMOVED_COLOR
        DocDiffStatus.CHANGED -> DIFF_CHANGED_COLOR
        DocDiffStatus.MOVED -> c.primary
        DocDiffStatus.SAME -> Color.Transparent
    }
    val badge = when (row.status) {
        DocDiffStatus.ADDED -> R.string.docs_history_status_added
        DocDiffStatus.REMOVED -> R.string.docs_history_status_removed
        DocDiffStatus.CHANGED -> R.string.docs_history_status_changed
        DocDiffStatus.MOVED -> R.string.docs_history_status_moved
        DocDiffStatus.SAME -> null
    }

    Column(
        Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 3.dp)
            .diffRail(rail)
            .padding(start = 8.dp),
    ) {
        badge?.let {
            Text(stringResource(it), color = c.text3, fontSize = 10.sp)
        }
        // The previous wording of an edited block is kept next to the new one:
        // «изменено» without the old text asks the reader to remember what they
        // came here to look up.
        if (row.prevText.isNotBlank()) {
            Text(
                row.prevText,
                color = c.text3,
                fontSize = 12.sp,
                textDecoration = TextDecoration.LineThrough,
            )
        }
        Text(
            row.text.ifBlank { "—" },
            color = if (row.status == DocDiffStatus.REMOVED || row.status == DocDiffStatus.SAME) c.text3 else c.text1,
            fontSize = 12.sp,
            textDecoration = if (row.status == DocDiffStatus.REMOVED) TextDecoration.LineThrough else null,
        )
    }
}

// The three status colours the web uses for the same rails (`--t-success`,
// `--t-error`, `--t-warning` in DocHistory.vue). They are literals rather than
// theme tokens because the neutral palette has no semantic colours — the same
// place the web takes them from a fallback.
private val DIFF_ADDED_COLOR = Color(0xFF18A058)
private val DIFF_REMOVED_COLOR = Color(0xFFD03050)
private val DIFF_CHANGED_COLOR = Color(0xFFF0A020)

private fun Modifier.diffRail(color: Color): Modifier =
    drawBehind { drawRect(color, size = Size(3.dp.toPx(), size.height)) }
