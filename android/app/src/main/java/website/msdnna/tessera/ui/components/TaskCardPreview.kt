package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.model.Tag
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraTheme
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewModel

/**
 * Standalone previews for iterating on the card visuals (esp. the priority
 * accent radius) in Android Studio — open this file and use the Split/Design
 * pane. Tweak [TaskCard]'s `leftAccent` and see it live here.
 *
 * The Russian in the fixtures below is sample *content* — a tag name, a task
 * title, a column name all arrive from the server and are never translated —
 * hence the `i18n-data` markers.
 */
private val sampleTags = listOf(
    Tag(id = "t1", name = "ЕПВВ", color = "#18a058"), // i18n-data
    Tag(id = "t2", name = "API", color = "#2f80ed"),
    Tag(id = "t3", name = "URGENT", color = "#e0533d"),
)
private val sampleMember = Member(userId = "u1", name = "MS")

private val parentTask = Task(
    id = "1", columnId = "c", title = "Развернуть внешние сервисы в ЗР с помощью Ansible", // i18n-data
    priority = 4, dueDate = "2026-06-04T00:00:00Z", number = 2,
    tagIds = listOf("t1", "t2", "t3"), assigneeIds = listOf("u1"),
)
private val subTask = Task(id = "s1", columnId = "c", parentId = "1", title = "test1", completedAt = "2026-06-04T00:00:00Z")
private val plainTask = Task(id = "2", columnId = "c", title = "test123", number = 5)

private fun sampleState() = BoardUiState(
    loading = false,
    columns = listOf(BoardColumn(id = "c", name = "В процессе")), // i18n-data
    tasks = listOf(parentTask, plainTask),
    subtasks = listOf(subTask),
    tags = sampleTags.associateBy { it.id },
    tagList = sampleTags,
    members = listOf(sampleMember),
)

@Composable
private fun PreviewCards(dark: Boolean) {
    TesseraTheme(isDark = dark) {
        val vm = remember { BoardViewModel() }
        val state = remember { sampleState() }
        Column(
            Modifier.width(300.dp).background(Tessera.colors.bg).padding(12.dp),
        ) {
            TaskCard(task = parentTask, state = state, vm = vm, onOpen = {})
            Spacer(Modifier.height(12.dp))
            TaskCard(task = plainTask, state = state, vm = vm, onOpen = {})
        }
    }
}

@Preview(name = "Card — light", showBackground = true)
@Composable
private fun TaskCardPreviewLight() = PreviewCards(dark = false)

@Preview(name = "Card — dark", showBackground = true)
@Composable
private fun TaskCardPreviewDark() = PreviewCards(dark = true)
