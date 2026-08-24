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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import website.msdnna.tessera.data.model.SearchNote
import website.msdnna.tessera.data.model.SearchTask
import website.msdnna.tessera.data.repository.HelpRepository
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.popupAppear
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.SearchViewModel
import website.msdnna.tessera.util.HelpHit
import website.msdnna.tessera.util.HelpSearcher
import website.msdnna.tessera.util.Ion

/** How many help articles the global search offers before the server's own
 *  results — enough to answer «как …», not enough to bury the tasks. */
private const val HELP_HITS_LIMIT = 4

/**
 * Full-screen search (web `SearchBar`, mobile-native): an autofocused field
 * over grouped help + task + note results. Picking a result navigates and closes.
 */
@Composable
fun SearchOverlay(
    workspaceId: String,
    onClose: () -> Unit,
    onOpenTask: (boardId: String, taskId: String) -> Unit,
    onOpenNote: (noteId: String) -> Unit,
    onOpenHelp: (slug: String) -> Unit = {},
) {
    val c = Tessera.colors
    val vm: SearchViewModel = viewModel()
    val state by vm.state.collectAsStateWithLifecycle()
    val focus = remember { FocusRequester() }

    // The manual is bundled, so its hits are ready on the keystroke — they show
    // while the server is still answering rather than after it (#2795).
    val assets = LocalContext.current.assets
    val searcher = remember(assets) { HelpSearcher(HelpRepository(assets).articles()) }
    val helpHits = remember(state.query, searcher) { searcher.search(state.query, HELP_HITS_LIMIT) }

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
                placeholder = "Поиск задач, заметок и справки",
                modifier = Modifier.weight(1f).focusRequester(focus),
            )
        }

        val results = state.results
        val serverEmpty = results?.isEmpty != false
        when {
            state.query.isBlank() -> Hint(
                "Введите запрос — поиск по названию и описанию задач, заголовку и тексту заметок, статьям справки",
            )

            helpHits.isEmpty() && serverEmpty && state.loading ->
                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { TesseraLoader() }

            helpHits.isEmpty() && results?.isEmpty == true -> Hint("Ничего не найдено")

            else -> LazyColumn(Modifier.fillMaxSize().padding(horizontal = 8.dp)) {
                if (helpHits.isNotEmpty()) {
                    item { SectionHeader("СПРАВКА") }
                    items(helpHits, key = { "h-${it.slug}" }) { hit ->
                        HelpResult(hit, onClick = { onOpenHelp(hit.slug) })
                    }
                }
                if (results != null) {
                    if (results.tasks.isNotEmpty()) {
                        item { SectionHeader("ЗАДАЧИ") }
                        items(results.tasks, key = { "t-${it.id}" }) { task ->
                            TaskResult(task, onClick = { onOpenTask(task.boardId, task.id) })
                        }
                    }
                    if (results.notes.isNotEmpty()) {
                        item { SectionHeader("ЗАМЕТКИ") }
                        items(results.notes, key = { "n-${it.id}" }) { note ->
                            NoteResult(note, onClick = { onOpenNote(note.id) })
                        }
                    }
                }
                // The help hits are already on screen while the server answers —
                // say that the rest is still coming instead of looking finished.
                if (state.loading) {
                    item {
                        Box(Modifier.fillMaxWidth().padding(vertical = 20.dp), contentAlignment = Alignment.Center) {
                            TesseraLoader()
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
private fun HelpResult(hit: HelpHit, onClick: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().clickableNoRipple(onClick = onClick).padding(horizontal = 8.dp, vertical = 11.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(Ion.HELP_CIRCLE, size = 18.dp, tint = c.text3)
        Spacer(Modifier.width(10.dp))
        Text(hit.title, color = c.text1, fontSize = 14.sp, maxLines = 1)
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
        Text(note.title.ifBlank { "Без названия" }, color = c.text1, fontSize = 14.sp, maxLines = 1)
    }
}
