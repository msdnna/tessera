package website.msdnna.tessera.ui.screens

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.SearchNote
import website.msdnna.tessera.data.model.SearchTask
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.popupAppear
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.SearchViewModel
import website.msdnna.tessera.util.Ion

/**
 * Full-screen search (web `SearchBar`, mobile-native): an autofocused field
 * over grouped task + note results. Picking a result navigates and closes.
 */
@Composable
fun SearchOverlay(
    workspaceId: String,
    onClose: () -> Unit,
    onOpenTask: (boardId: String, taskId: String) -> Unit,
    onOpenNote: (noteId: String) -> Unit,
) {
    val c = Tessera.colors
    val vm: SearchViewModel = viewModel()
    val state by vm.state.collectAsStateWithLifecycle()
    val focus = remember { FocusRequester() }

    LaunchedEffect(workspaceId) { vm.bind(workspaceId) }
    LaunchedEffect(Unit) { focus.requestFocus() }

    // This overlay is a plain Box (not a Dialog), so Back would otherwise fall
    // through to the nav back-stack — intercept it to just close the search.
    BackHandler { onClose() }

    Column(
        Modifier.popupAppear(TransformOrigin.Center).fillMaxSize().background(c.bg).windowInsetsPadding(WindowInsets.safeDrawing),
    ) {
        Row(
            Modifier.fillMaxWidth().background(c.surface).padding(horizontal = 8.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IonIconButton(Ion.CLOSE, onClick = onClose, boxSize = 40.dp)
            Spacer(Modifier.width(4.dp))
            TTextField(
                value = state.query,
                onValueChange = vm::onQueryChange,
                placeholder = stringResource(R.string.search_placeholder),
                modifier = Modifier.weight(1f).focusRequester(focus),
            )
        }

        when {
            state.loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                TesseraLoader()
            }

            state.query.isBlank() -> Hint(stringResource(R.string.search_hint))

            state.results?.isEmpty == true -> Hint(stringResource(R.string.search_empty))

            else -> {
                val results = state.results
                if (results != null) {
                    LazyColumn(Modifier.fillMaxSize().padding(horizontal = 8.dp)) {
                        if (results.tasks.isNotEmpty()) {
                            item { SectionHeader(stringResource(R.string.search_section_tasks)) }
                            items(results.tasks, key = { "t-${it.id}" }) { task ->
                                TaskResult(task, onClick = { onOpenTask(task.boardId, task.id) })
                            }
                        }
                        if (results.notes.isNotEmpty()) {
                            item { SectionHeader(stringResource(R.string.search_section_notes)) }
                            items(results.notes, key = { "n-${it.id}" }) { note ->
                                NoteResult(note, onClick = { onOpenNote(note.id) })
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun Hint(text: String) {
    Box(Modifier.fillMaxSize().padding(32.dp), contentAlignment = Alignment.Center) {
        Text(text, color = Tessera.colors.text3, fontSize = 14.sp)
    }
}

@Composable
private fun SectionHeader(label: String) {
    Text(
        label,
        color = Tessera.colors.text3,
        fontSize = 11.sp,
        fontWeight = FontWeight.SemiBold,
        modifier = Modifier.padding(start = 8.dp, top = 14.dp, bottom = 4.dp),
    )
}

@Composable
private fun TaskResult(task: SearchTask, onClick: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().clickableNoRipple(onClick = onClick).padding(horizontal = 8.dp, vertical = 11.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(
            if (task.isCompleted) Ion.CHECK_CIRCLE else Ion.ELLIPSE,
            size = 18.dp,
            tint = if (task.isCompleted) c.primary else c.text3,
            gradient = task.isCompleted,
        )
        Spacer(Modifier.width(10.dp))
        task.number?.let {
            Text("#$it", color = c.text3, fontSize = 13.sp)
            Spacer(Modifier.width(6.dp))
        }
        Text(task.title, color = c.text1, fontSize = 14.sp, maxLines = 1, modifier = Modifier.weight(1f))
    }
}

@Composable
private fun NoteResult(note: SearchNote, onClick: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().clickableNoRipple(onClick = onClick).padding(horizontal = 8.dp, vertical = 11.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(Ion.DOCUMENT_TEXT, size = 18.dp, tint = c.text3)
        Spacer(Modifier.width(10.dp))
        Text(
            note.title.ifBlank { stringResource(R.string.notes_untitled) },
            color = c.text1,
            fontSize = 14.sp,
            maxLines = 1,
        )
    }
}
