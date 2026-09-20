package website.msdnna.tessera.ui.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.unit.dp
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.data.model.Attachment
import website.msdnna.tessera.data.model.Comment
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.theme.TesseraTheme

/**
 * Первый слой архивного режима модалки (#2932): кнопки, которых там быть не
 * должно, не рисуются вовсе.
 *
 * Второй слой — отказ самой воронки правок — в `TaskDetailReadOnlyTest`. Здесь
 * проверяются две строки, которые в архиве расходятся сильнее всего:
 *
 *  * свой комментарий: ✎ и ✕ уходят, но текст остаётся читаемым;
 *  * вложение: «скачать» остаётся (за этим архивную задачу чаще всего и
 *    открывают), «удалить» уходит.
 *
 * Размер экрана пришпилен, как в [TaskCommentRowTest]: на дефолтных 320×470px
 * тап ниже кромки теряется молча.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class TaskArchiveReadOnlyUiTest {
    @get:Rule
    val compose = createComposeRule()

    private var deleted = 0
    private var downloaded = 0

    private val mine = Comment(
        id = "c1",
        taskId = "t1",
        authorId = "me",
        authorName = "Я",
        body = "мой комментарий",
        createdAt = "2026-09-20T09:00:00Z",
    )

    private val attachment = Attachment(id = "a1", taskId = "t1", filename = "смета.pdf", size = 2048)

    private fun mountComment(editable: Boolean) {
        compose.setContent {
            TesseraTheme {
                CommentRow(
                    cm = mine,
                    avatarSize = 26.dp,
                    meId = "me",
                    mentionItems = emptyList(),
                    onTaskRef = {},
                    editing = false,
                    editBody = "",
                    onEditBody = {},
                    editPreview = false,
                    onToggleEditPreview = {},
                    onStartEdit = {},
                    onEndEdit = {},
                    onSaveEdit = {},
                    onToggleCheck = {},
                    onDelete = { deleted += 1 },
                    editable = editable,
                    actions = {},
                )
            }
        }
    }

    private fun mountAttachment(canDelete: Boolean) {
        compose.setContent {
            TesseraTheme {
                AttachmentRow(
                    attachment = attachment,
                    onDownload = { downloaded += 1 },
                    onDelete = { deleted += 1 },
                    canDelete = canDelete,
                )
            }
        }
    }

    @Test
    fun `в архиве свой комментарий нечем удалить`() {
        mountComment(editable = false)

        compose.onNodeWithTag(TestTags.taskCommentDelete("c1")).assertDoesNotExist()
        compose.onNodeWithTag(TestTags.TASK_COMMENT_DELETE_CONFIRM).assertDoesNotExist()
        assertThat(deleted).isEqualTo(0)
    }

    @Test
    fun `на живой задаче свой комментарий по-прежнему правится`() {
        // Контроль: режим снимает кнопки ровно в архиве, а не у всех подряд.
        mountComment(editable = true)

        compose.onNodeWithTag(TestTags.taskCommentDelete("c1")).assertIsDisplayed()
    }

    @Test
    fun `в архиве вложение скачивается, но не удаляется`() {
        mountAttachment(canDelete = false)

        compose.onNodeWithTag(TestTags.taskAttachmentDelete("a1")).assertDoesNotExist()
        compose.onNodeWithTag(TestTags.taskAttachmentDownload("a1")).performClick()

        assertThat(downloaded).isEqualTo(1)
        assertThat(deleted).isEqualTo(0)
    }

    @Test
    fun `на живой задаче вложение удаляется через подтверждение`() {
        // Второй контроль: обе кнопки на месте, и разрушительная всё так же
        // спрашивает — архивный режим не должен был этого менять.
        mountAttachment(canDelete = true)

        compose.onNodeWithTag(TestTags.taskAttachmentDownload("a1")).assertIsDisplayed()
        compose.onNodeWithTag(TestTags.taskAttachmentDelete("a1")).performClick()

        assertThat(deleted).isEqualTo(0)
    }
}
