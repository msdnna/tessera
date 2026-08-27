package website.msdnna.tessera.ui.screens

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.Note
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.NotesViewModel
import website.msdnna.tessera.util.Ion

/**
 * Notes module (web `NotesView`), mobile master/detail: a note list, with the
 * editor sliding over it when a note is opened or created. Save persists; the
 * body is plain Markdown text.
 */
@Composable
fun NotesScreen(
    workspaceId: String,
    preselectNoteId: String?,
    onPreselectConsumed: () -> Unit,
) {
    val c = Tessera.colors
    val vm: NotesViewModel = viewModel()
    val state by vm.state.collectAsStateWithLifecycle()

    LaunchedEffect(workspaceId, preselectNoteId) {
        if (workspaceId.isNotBlank()) vm.load(workspaceId, preselectNoteId)
        if (preselectNoteId != null) onPreselectConsumed()
    }

    val editorOpen = state.selectedId != null || state.composingNew

    // The note editor is an inline overlay (not a Dialog), so Back would fall
    // through to the nav back-stack — intercept it to close the editor instead.
    BackHandler(enabled = editorOpen) { vm.closeEditor() }

    Box(Modifier.fillMaxSize().background(c.bg)) {
        Column(Modifier.fillMaxSize()) {
            Row(
                Modifier.fillMaxWidth().padding(12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                TButton(stringResource(R.string.notes_new), onClick = { vm.newNote() })
            }
            when {
                state.loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    TesseraLoader()
                }

                state.list.isEmpty() -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        IonIcon(Ion.DOCUMENT_TEXT, size = 40.dp, tint = c.text3)
                        Spacer(Modifier.height(10.dp))
                        Text(stringResource(R.string.notes_empty), color = c.text3, fontSize = 14.sp)
                    }
                }

                else -> LazyColumn(
                    Modifier.fillMaxSize().padding(horizontal = 12.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    items(state.list, key = { it.id }) { note ->
                        NoteListItem(note, onClick = { vm.select(note) })
                    }
                    item { Spacer(Modifier.height(16.dp)) }
                }
            }
        }

        if (editorOpen) {
            NoteEditor(
                note = state.selected,
                onSave = { title, body -> vm.save(title, body) },
                onDelete = { state.selected?.let { vm.delete(it.id) } },
                onBack = { vm.closeEditor() },
            )
        }
    }
}

@Composable
private fun NoteListItem(note: Note, onClick: () -> Unit) {
    val c = Tessera.colors
    Column(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).background(c.cardSurface)
            .border(1.dp, c.border, RoundedCornerShape(RadiusMd)).clickableNoRipple(onClick = onClick)
            .padding(12.dp),
    ) {
        Text(
            note.title.ifBlank { stringResource(R.string.notes_untitled) },
            color = c.text1,
            fontSize = 15.sp,
            fontWeight = FontWeight.SemiBold,
            maxLines = 1,
        )
        val snippet = note.body.replace("\n", " ").take(80)
        if (snippet.isNotBlank()) {
            Spacer(Modifier.height(3.dp))
            Text(snippet, color = c.text3, fontSize = 12.sp, maxLines = 1)
        }
    }
}

@Composable
private fun NoteEditor(
    note: Note?,
    onSave: (title: String, body: String) -> Unit,
    onDelete: () -> Unit,
    onBack: () -> Unit,
) {
    val c = Tessera.colors
    var title by remember(note?.id) { mutableStateOf(note?.title ?: "") }
    var body by remember(note?.id) { mutableStateOf(note?.body ?: "") }
    var confirmDelete by remember { mutableStateOf(false) }
    // decorationBox — обычная лямбда, не композабл: плейсхолдеры берём заранее.
    val titlePlaceholder = stringResource(R.string.notes_title_placeholder)
    val bodyPlaceholder = stringResource(R.string.notes_body_placeholder)

    Column(Modifier.fillMaxSize().background(c.surface)) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IonIconButton(
                Ion.CHEVRON_FORWARD,
                onClick = onBack,
                boxSize = 40.dp,
                modifier = Modifier.graphicsLayer { scaleX = -1f },
            )
            Spacer(Modifier.width(4.dp))
            Text(
                stringResource(if (note == null) R.string.notes_new else R.string.notes_editor_title),
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.weight(1f),
            )
            if (note != null) {
                Box {
                    IonIconButton(Ion.TRASH, onClick = { confirmDelete = true }, boxSize = 36.dp, tint = c.text3)
                    TConfirmPopover(
                        expanded = confirmDelete,
                        message = stringResource(R.string.notes_delete_confirm),
                        onConfirm = {
                            confirmDelete = false
                            onDelete()
                        },
                        onDismiss = { confirmDelete = false },
                    )
                }
            }
        }
        HorizontalDivider(color = c.border)

        BasicTextField(
            value = title,
            onValueChange = { title = it },
            singleLine = true,
            textStyle = TextStyle(color = c.text1, fontSize = 18.sp, fontWeight = FontWeight.SemiBold),
            cursorBrush = SolidColor(c.primary),
            modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 14.dp),
            decorationBox = { inner ->
                if (title.isEmpty()) Text(titlePlaceholder, color = c.placeholder, fontSize = 18.sp, fontWeight = FontWeight.SemiBold)
                inner()
            },
        )
        HorizontalDivider(color = c.border)

        BasicTextField(
            value = body,
            onValueChange = { body = it },
            textStyle = TextStyle(color = c.text1, fontSize = 14.sp),
            cursorBrush = SolidColor(c.primary),
            modifier = Modifier.fillMaxWidth().weight(1f).padding(16.dp),
            decorationBox = { inner ->
                if (body.isEmpty()) Text(bodyPlaceholder, color = c.placeholder, fontSize = 14.sp)
                inner()
            },
        )

        HorizontalDivider(color = c.border)
        Row(
            Modifier.fillMaxWidth().padding(12.dp),
            horizontalArrangement = Arrangement.End,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            TButton(stringResource(R.string.common_cancel), kind = TButtonKind.Ghost, onClick = onBack)
            Spacer(Modifier.width(8.dp))
            TButton(stringResource(R.string.common_save), enabled = title.isNotBlank(), onClick = { onSave(title.trim(), body) })
        }
    }
}
