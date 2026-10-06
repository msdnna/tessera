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
import website.msdnna.tessera.data.model.Comment
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.theme.TesseraTheme

/**
 * Что делает «✕» под своим комментарием задачи.
 *
 * Раньше он звал удаление сразу — единственная разрушительная кнопка приложения
 * без вопроса: рядом спрашивают и архив/удаление задачи, и снятие связи, и
 * удаление комментария документа, и веб (`n-popconfirm` на корне и на ответе).
 * Отката нет ни у кого, а на корне ветки сервер вдобавок поднимает чужой ответ
 * в корень — промах по иконке 26dp перестраивал чужую переписку.
 *
 * Размер экрана пришпилен, как в [ConferenceChatSheetTest]: на дефолтных
 * 320×470px тап ниже кромки теряется молча.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "ru-w411dp-h891dp-xhdpi")
class TaskCommentRowTest {
    @get:Rule
    val compose = createComposeRule()

    private var deleted = 0
    private var saved = ""

    private fun comment(id: String, authorId: String? = "me") = Comment(
        id = id,
        taskId = "t1",
        authorId = authorId,
        authorName = "Я",
        body = "текст $id",
        createdAt = "2026-09-19T03:00:00Z",
    )

    private fun mount(cm: Comment, meId: String? = "me") {
        compose.setContent {
            TesseraTheme {
                CommentRow(
                    cm = cm,
                    avatarSize = 26.dp,
                    meId = meId,
                    mentionItems = emptyList(),
                    onTaskRef = {},
                    editing = false,
                    editBody = "",
                    onEditBody = {},
                    editPreview = false,
                    onToggleEditPreview = {},
                    onStartEdit = {},
                    onEndEdit = {},
                    onSaveEdit = { saved = it },
                    onToggleCheck = {},
                    onDelete = { deleted += 1 },
                    actions = {},
                )
            }
        }
    }

    @Test
    fun `нажатие на крестик спрашивает, а не удаляет`() {
        mount(comment("c1"))

        compose.onNodeWithTag(TestTags.taskCommentDelete("c1")).performClick()

        // Попап на экране, а до удаления нажим не дошёл: отката у него нет.
        compose.onNodeWithTag(TestTags.TASK_COMMENT_DELETE_CONFIRM).assertIsDisplayed()
        assertThat(deleted).isEqualTo(0)
    }

    @Test
    fun `удаление доходит только через подтверждение`() {
        mount(comment("c1"))

        compose.onNodeWithTag(TestTags.taskCommentDelete("c1")).performClick()
        compose.onNodeWithTag(TestTags.TASK_COMMENT_DELETE_CONFIRM).performClick()

        assertThat(deleted).isEqualTo(1)
        // И попап закрылся — иначе второе «да» удалило бы уже не тот комментарий.
        compose.onNodeWithTag(TestTags.TASK_COMMENT_DELETE_CONFIRM).assertDoesNotExist()
    }

    @Test
    fun `чужой комментарий нечем удалить`() {
        // Характеризует неизменённое: кнопки редактирования и удаления рисуются
        // только автору, и подтверждение этого не меняет.
        mount(comment("c1", authorId = "them"))

        compose.onNodeWithTag(TestTags.taskCommentDelete("c1")).assertDoesNotExist()
        compose.onNodeWithTag(TestTags.TASK_COMMENT_DELETE_CONFIRM).assertDoesNotExist()
    }
}
