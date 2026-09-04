package website.msdnna.tessera.ui.screens.documents

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.TConfirmDialog
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TInputDialog
import website.msdnna.tessera.ui.components.TMenuDivider
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.util.Ion

/**
 * What the composer is asking about. One value rather than a flag per dialog:
 * the questions are mutually exclusive, and a delete confirmation that can be
 * open at the same time as a rename is a way to delete the wrong document.
 */
sealed interface DocDraft {
    /** A new document at the level the grid is showing. */
    data object Create : DocDraft

    /** A new document under the open one. */
    data object Nested : DocDraft

    data class Rename(val title: String) : DocDraft

    /** [children] is what the confirmation is worded with, and what decides
     *  whether the delete has to be recursive. */
    data class Remove(val children: Int) : DocDraft
}

/**
 * The create / rename / delete dialogs of the documents section (#2894).
 *
 * The web creates a document titled «Без названия» straight away and lets the
 * user rename it in the editor's title field. Android has no editor yet (§4),
 * so the title is asked for here — prefilled with the same default, so pressing
 * through without typing lands on exactly the web's behaviour.
 */
@Composable
fun DocumentComposer(
    draft: DocDraft?,
    onDismiss: () -> Unit,
    onCreate: (String) -> Unit,
    onNested: (String) -> Unit,
    onRename: (String) -> Unit,
    onRemove: () -> Unit,
) {
    val untitled = stringResource(R.string.docs_untitled)
    when (draft) {
        null -> Unit

        DocDraft.Create -> TitleDialog(
            title = stringResource(R.string.docs_create),
            initial = untitled,
            confirmText = stringResource(R.string.docs_create_confirm),
            onConfirm = onCreate,
            onDismiss = onDismiss,
        )

        DocDraft.Nested -> TitleDialog(
            title = stringResource(R.string.docs_action_nested),
            initial = untitled,
            confirmText = stringResource(R.string.docs_create_confirm),
            onConfirm = onNested,
            onDismiss = onDismiss,
        )

        is DocDraft.Rename -> TitleDialog(
            title = stringResource(R.string.docs_rename_title),
            initial = draft.title,
            confirmText = stringResource(R.string.common_save),
            onConfirm = onRename,
            onDismiss = onDismiss,
        )

        is DocDraft.Remove -> TConfirmDialog(
            title = stringResource(R.string.docs_remove_title),
            message = if (draft.children > 0) {
                stringResource(R.string.docs_remove_with_children, draft.children)
            } else {
                stringResource(R.string.docs_remove_single)
            },
            confirmTag = TestTags.DOCUMENT_REMOVE_CONFIRM,
            onConfirm = onRemove,
            onDismiss = onDismiss,
        )
    }
}

@Composable
private fun TitleDialog(
    title: String,
    initial: String,
    confirmText: String,
    onConfirm: (String) -> Unit,
    onDismiss: () -> Unit,
) = TInputDialog(
    title = title,
    initial = initial,
    confirmText = confirmText,
    placeholder = stringResource(R.string.docs_untitled),
    fieldTag = TestTags.DOCUMENT_TITLE_INPUT,
    confirmTag = TestTags.DOCUMENT_TITLE_CONFIRM,
    onConfirm = onConfirm,
    onDismiss = onDismiss,
)

/**
 * The reader's «⋯» menu — the web's «Действия» dropdown: nest, walk into the
 * children, rename, delete. «Показать вложенные» appears only when there are
 * any, as on the web: an item that leads to an empty level is a dead end.
 */
@Composable
fun DocumentActionsMenu(
    expanded: Boolean,
    childCount: Int,
    onDismiss: () -> Unit,
    onNested: () -> Unit,
    onChildren: () -> Unit,
    onRename: () -> Unit,
    onRemove: () -> Unit,
) = TDropdown(expanded = expanded, onDismiss = onDismiss) {
    TMenuItem(
        stringResource(R.string.docs_action_nested),
        icon = Ion.ADD,
        onClick = onNested,
        modifier = Modifier.testTag(TestTags.DOCUMENT_ACTION_NESTED),
    )
    if (childCount > 0) {
        TMenuItem(
            stringResource(R.string.docs_action_children, childCount),
            icon = Ion.FOLDER,
            onClick = onChildren,
            modifier = Modifier.testTag(TestTags.DOCUMENT_ACTION_CHILDREN),
        )
    }
    TMenuItem(
        stringResource(R.string.docs_action_rename),
        icon = Ion.PENCIL,
        onClick = onRename,
        modifier = Modifier.testTag(TestTags.DOCUMENT_ACTION_RENAME),
    )
    TMenuDivider()
    TMenuItem(
        stringResource(R.string.docs_action_remove),
        icon = Ion.TRASH,
        danger = true,
        onClick = onRemove,
        modifier = Modifier.testTag(TestTags.DOCUMENT_ACTION_REMOVE),
    )
}
