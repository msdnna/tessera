package website.msdnna.tessera.ui.screens.documents

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TMenuDivider
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.Ion

/**
 * A document to jump to from the open one's menu: the one it hangs off, and the
 * ones hanging off it.
 */
data class DocSwitchRow(
    val id: String,
    val title: String,
    val icon: String,
    /** True for the document above this one, false for a nested one. */
    val parent: Boolean,
)

/** An item of the title menu that does one thing and needs nothing said back. */
enum class DocAction {
    /** Start editing. */
    EDIT,

    /** Stop editing and read the result. */
    READ,
    COMMENTS,
    LINKS,
    HISTORY,
    TOC,
    NESTED,
    CHILDREN,
    RENAME,
    REMOVE,

    /** Leave the document for the list of them. */
    LIST,
}

/** What the bar *shows*: the document, as much of it as a title needs to know. */
data class DocChromeInfo(
    val title: String,
    val icon: String,
    /** The editor is up, so the menu offers going back to reading and the bar
     *  carries the autosave status. */
    val editing: Boolean,
    /** Autosave status as a string resource; null while merely reading. */
    val status: Int?,
    val statusSettled: Boolean,
    val commentCount: Int,
    val linkCount: Int,
    val childCount: Int,
    val rows: List<DocSwitchRow>,
    val exportFormats: List<String>,
)

/**
 * What the shell's top bar shows while a document is open (#2894 rework).
 *
 * The section used to draw a header of its own under the app's, and the two
 * said the same thing twice: «Документы» over «Техническое задание», a back
 * arrow over a hamburger, and six icon buttons squeezed into the width left.
 * So the document takes the shell's bar over: its name replaces the section's,
 * and everything the second row held moves into the menu behind that name.
 *
 * The plain items go through one [DocAction] callback rather than a lambda
 * each: they are a menu, and a menu is a list of choices, not two dozen
 * separate wires. The three that carry an argument keep their own.
 *
 * Not a data class — it is mostly callbacks, and structural equality over
 * lambdas is meaningless.
 */
class DocChrome(
    val info: DocChromeInfo,
    val onAction: (DocAction) -> Unit,
    val onExport: (String) -> Unit,
    val onSwitch: (DocSwitchRow) -> Unit,
    val onRemoveRow: (DocSwitchRow) -> Unit,
)

/**
 * The open document's name in the shell's top bar, with everything that used to
 * live in the section's own header behind it.
 *
 * Shaped after the board switcher next to it, because it is the same gesture:
 * the title of the thing on screen is also the way to act on it and to step to
 * its neighbours.
 */
@Composable
fun DocTitleSwitcher(chrome: DocChrome, modifier: Modifier = Modifier) {
    val c = Tessera.colors
    val info = chrome.info
    var menu by remember { mutableStateOf(false) }
    var exportOpen by remember { mutableStateOf(false) }
    Box(modifier) {
        Row(
            Modifier.clickableNoRipple { menu = true }.testTag(TestTags.DOCUMENT_MENU),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            if (info.icon.isNotBlank()) {
                Text(info.icon, fontSize = 16.sp)
                Spacer(Modifier.width(6.dp))
            }
            Column(Modifier.weight(1f, fill = false)) {
                Text(
                    info.title.ifBlank { stringResource(R.string.docs_reader_untitled) },
                    color = c.text1,
                    fontSize = 17.sp,
                    fontWeight = FontWeight.SemiBold,
                    maxLines = 1,
                )
                // Autosave is the one thing the editor cannot leave unsaid, and
                // its row is gone — so it rides under the title rather than
                // costing the bar a line of its own.
                info.status?.let { label ->
                    Text(
                        stringResource(label),
                        color = if (info.statusSettled) c.text3 else c.text2,
                        fontSize = 11.sp,
                        maxLines = 1,
                        modifier = Modifier.testTag(TestTags.DOCUMENT_EDITOR_STATUS),
                    )
                }
            }
            Spacer(Modifier.width(4.dp))
            IonIcon(Ion.CHEVRON_DOWN, size = 18.dp, tint = c.text2)
        }
        DocMenu(
            chrome = chrome,
            expanded = menu,
            onAct = { action ->
                menu = false
                chrome.onAction(action)
            },
            onDismiss = { menu = false },
            onExportPick = {
                menu = false
                exportOpen = true
            },
        )
        // A sibling of the menu, not a child: TDropdown anchors on this Box, and
        // a submenu inside a popover that is closing has nothing to hang off.
        DocExportMenu(
            expanded = exportOpen,
            formats = info.exportFormats,
            onDismiss = { exportOpen = false },
            onPick = { format ->
                exportOpen = false
                chrome.onExport(format)
            },
        )
    }
}

@Composable
private fun DocMenu(
    chrome: DocChrome,
    expanded: Boolean,
    onAct: (DocAction) -> Unit,
    onDismiss: () -> Unit,
    onExportPick: () -> Unit,
) = TDropdown(expanded = expanded, onDismiss = onDismiss, scrollable = true) {
    val info = chrome.info
    Column(Modifier.widthIn(min = 248.dp)) {
        // Reading and writing are the two states of this screen, so the item
        // that swaps them comes first: it is the one reached most often.
        if (info.editing) {
            MenuAction(R.string.docs_menu_read, Ion.EYE, TestTags.DOCUMENT_EDITOR_CLOSE, DocAction.READ, onAct)
        } else {
            MenuAction(R.string.docs_menu_edit, Ion.PENCIL, TestTags.DOCUMENT_EDIT, DocAction.EDIT, onAct)
        }
        TMenuItem(
            countedLabel(stringResource(R.string.docs_menu_comments), info.commentCount),
            icon = Ion.CHATBUBBLE,
            onClick = { onAct(DocAction.COMMENTS) },
            modifier = Modifier.testTag(TestTags.DOCUMENT_COMMENTS_OPEN),
        )
        TMenuItem(
            countedLabel(stringResource(R.string.docs_menu_links), info.linkCount),
            icon = Ion.LINK,
            onClick = { onAct(DocAction.LINKS) },
            modifier = Modifier.testTag(TestTags.DOCUMENT_LINKS_OPEN),
        )
        MenuAction(R.string.docs_menu_history, Ion.TIME, TestTags.DOCUMENT_HISTORY_OPEN, DocAction.HISTORY, onAct)
        // The outline walks the text on screen, and the editor draws its own.
        if (!info.editing) {
            MenuAction(R.string.docs_toc_title, Ion.LIST, TestTags.DOCUMENT_TOC_OPEN, DocAction.TOC, onAct)
        }

        TMenuDivider()
        MenuAction(R.string.docs_action_nested, Ion.ADD, TestTags.DOCUMENT_ACTION_NESTED, DocAction.NESTED, onAct)
        if (info.childCount > 0) {
            TMenuItem(
                stringResource(R.string.docs_action_children, info.childCount),
                icon = Ion.FOLDER,
                onClick = { onAct(DocAction.CHILDREN) },
                modifier = Modifier.testTag(TestTags.DOCUMENT_ACTION_CHILDREN),
            )
        }
        MenuAction(R.string.docs_action_rename, Ion.PENCIL, TestTags.DOCUMENT_ACTION_RENAME, DocAction.RENAME, onAct)
        TMenuItem(
            stringResource(R.string.docs_export),
            icon = Ion.DOWNLOAD,
            onClick = onExportPick,
            modifier = Modifier.testTag(TestTags.DOCUMENT_ACTION_EXPORT),
        )
        TMenuItem(
            stringResource(R.string.docs_action_remove),
            icon = Ion.TRASH,
            danger = true,
            onClick = { onAct(DocAction.REMOVE) },
            modifier = Modifier.testTag(TestTags.DOCUMENT_ACTION_REMOVE),
        )

        if (info.rows.isNotEmpty()) {
            TMenuDivider()
            SectionLabel(stringResource(R.string.docs_menu_switch))
            info.rows.forEach { row -> SwitchRow(chrome, row) }
        }
        TMenuDivider()
        MenuAction(R.string.docs_menu_list, Ion.ALBUMS, TestTags.DOCUMENT_BACK, DocAction.LIST, onAct)
    }
}

@Composable
private fun MenuAction(label: Int, icon: String, tag: String, action: DocAction, onAct: (DocAction) -> Unit) {
    TMenuItem(
        stringResource(label),
        icon = icon,
        onClick = { onAct(action) },
        modifier = Modifier.testTag(tag),
    )
}

/**
 * One neighbouring document: its name opens it, the bin beside it deletes it.
 *
 * The bin is its own tappable box rather than a trailing icon of the row —
 * a row that both navigates and deletes on the same tap target is a row that
 * deletes by accident.
 */
@Composable
private fun SwitchRow(chrome: DocChrome, row: DocSwitchRow) {
    val c = Tessera.colors
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Row(
            Modifier.weight(1f)
                .clickableNoRipple { chrome.onSwitch(row) }
                .padding(start = 14.dp, top = 10.dp, bottom = 10.dp, end = 4.dp)
                .testTag(TestTags.DOCUMENT_SWITCH_ROW),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            // The document above is a different kind of step from the ones
            // below it, and the icon is what says which without a second label.
            IonIcon(if (row.parent) Ion.FOLDER else Ion.DOCUMENT_TEXT, size = 16.dp, tint = c.text3)
            Spacer(Modifier.width(8.dp))
            if (row.icon.isNotBlank()) {
                Text(row.icon, fontSize = 13.sp)
                Spacer(Modifier.width(4.dp))
            }
            Text(
                row.title.ifBlank { stringResource(R.string.docs_reader_untitled) },
                color = c.text1,
                fontSize = 13.sp,
                maxLines = 1,
            )
        }
        Box(
            Modifier
                .testTag(TestTags.DOCUMENT_SWITCH_REMOVE)
                .clickableNoRipple { chrome.onRemoveRow(row) }
                .padding(horizontal = 12.dp, vertical = 10.dp),
        ) {
            IonIcon(Ion.TRASH, size = 16.dp, tint = c.text3)
        }
    }
}

@Composable
private fun SectionLabel(text: String) {
    Text(
        text,
        color = Tessera.colors.text3,
        fontSize = 11.sp,
        fontWeight = FontWeight.SemiBold,
        modifier = Modifier.padding(horizontal = 14.dp, vertical = 4.dp),
    )
}

/** «Обсуждения · 3». A count of zero says nothing worth the width. */
private fun countedLabel(label: String, count: Int): String = if (count > 0) "$label · $count" else label
