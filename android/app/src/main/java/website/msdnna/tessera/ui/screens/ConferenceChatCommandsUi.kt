package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.ConferenceMessage
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.MemberAvatar
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.ConferenceChatUiState
import website.msdnna.tessera.util.ChatMember
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.chatCommandMatches
import website.msdnna.tessera.util.chatMentionMatches
import website.msdnna.tessera.util.chatMentionQuery
import website.msdnna.tessera.util.chatSlashQuery
import website.msdnna.tessera.util.chatSplitMentions

// The call chat's «/» and «@» on Android (#3055, web ConferenceChat.vue).

/** A line with every «@Имя» of a member drawn as a chip; the reader's own a shade stronger. */
@Composable
internal fun mentionText(body: String, members: List<ChatMember>, meId: String): AnnotatedString {
    val c = Tessera.colors
    return buildAnnotatedString {
        chatSplitMentions(body, members).forEach { part ->
            val m = part.member
            if (m == null) {
                append(part.text)
            } else {
                val alpha = if (m.userId == meId) 0.26f else 0.14f
                withStyle(
                    SpanStyle(color = c.primary, fontWeight = FontWeight.SemiBold, background = c.primary.copy(alpha = alpha)),
                ) { append(part.text) }
            }
        }
    }
}

/** A task line: «Мария создал(а) задачу #2978 Прогноз» — no bubble. */
@Composable
internal fun TaskLine(message: ConferenceMessage) {
    val c = Tessera.colors
    val name = message.userName?.takeIf { it.isNotBlank() } ?: stringResource(R.string.conf_chat_unknown_author)
    val created = stringResource(R.string.conf_chat_task_created)
    val task = message.task
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 4.dp)
            .clip(RoundedCornerShape(10.dp)).background(c.primary.copy(alpha = 0.08f))
            .padding(horizontal = 10.dp, vertical = 7.dp)
            .testTag(TestTags.CONFERENCE_CHAT_TASK_LINE),
        verticalAlignment = Alignment.Top,
    ) {
        IonIcon(Ion.ADD_CIRCLE, size = 15.dp, tint = c.primary)
        Spacer(Modifier.width(8.dp))
        Text(
            buildAnnotatedString {
                withStyle(SpanStyle(color = c.text1, fontWeight = FontWeight.Medium)) { append(name) }
                append(" $created ")
                if (task != null) {
                    task.number?.let { withStyle(SpanStyle(color = c.text3)) { append("#$it ") } }
                    withStyle(SpanStyle(color = c.text1, fontWeight = FontWeight.SemiBold)) { append(task.title) }
                } else {
                    withStyle(SpanStyle(color = c.text3, textDecoration = TextDecoration.LineThrough)) {
                        append(message.body)
                    }
                }
            },
            color = c.text2,
            fontSize = 12.5.sp,
            modifier = Modifier.weight(1f),
        )
    }
}

/** The «/» and «@» list over the composer: a tap fills «/задача » or «@Имя ». */
@Composable
internal fun ChatSuggest(state: ConferenceChatUiState, onDraft: (String) -> Unit) {
    val c = Tessera.colors
    val draft = state.draft
    val slash = chatSlashQuery(draft)
    val cmdWord = stringResource(R.string.conf_chat_cmd_task)
    val mention = if (slash == null) chatMentionQuery(draft) else null
    val people = mention?.let { chatMentionMatches(state.members, it.second, state.meId) }.orEmpty()
    val showCmd = slash != null && chatCommandMatches(slash)
    if (!showCmd && people.isEmpty()) return
    Column(
        Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 4.dp)
            .clip(RoundedCornerShape(12.dp)).background(c.surfaceAlt)
            .padding(vertical = 4.dp)
            .testTag(TestTags.CONFERENCE_CHAT_SUGGEST),
    ) {
        Text(
            stringResource(if (showCmd) R.string.conf_chat_cmd_head else R.string.conf_chat_mention_head),
            color = c.text3,
            fontSize = 11.sp,
            fontWeight = FontWeight.SemiBold,
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 3.dp),
        )
        if (showCmd) {
            Row(
                Modifier.fillMaxWidth().clickableNoRipple { onDraft("/$cmdWord ") }
                    .padding(horizontal = 12.dp, vertical = 8.dp)
                    .testTag(TestTags.conferenceChatSuggestRow("task")),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("/$cmdWord", color = c.text1, fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
                Spacer(Modifier.width(8.dp))
                Text(stringResource(R.string.conf_chat_cmd_task_hint), color = c.text3, fontSize = 12.sp)
            }
        } else {
            val start = mention?.first ?: 0
            people.forEach { m ->
                Row(
                    Modifier.fillMaxWidth()
                        .clickableNoRipple { onDraft(draft.substring(0, start) + "@" + m.name + " ") }
                        .padding(horizontal = 12.dp, vertical = 7.dp)
                        .testTag(TestTags.conferenceChatSuggestRow(m.userId)),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    MemberAvatar(20.dp, m.name, userId = m.userId)
                    Spacer(Modifier.width(8.dp))
                    Text(m.name, color = c.text1, fontSize = 13.sp, maxLines = 1)
                    Spacer(Modifier.width(8.dp))
                    Text(m.email, color = c.text3, fontSize = 12.sp, maxLines = 1)
                }
            }
        }
    }
}

/** «Задача в [доска]» while the draft is «/задача …». */
@Composable
internal fun TaskStrip(state: ConferenceChatUiState, onPickBoard: (String) -> Unit) {
    val c = Tessera.colors
    val title = state.taskTitle ?: return
    var open by remember { mutableStateOf(false) }
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 4.dp)
            .testTag(TestTags.CONFERENCE_CHAT_TASK_STRIP),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IonIcon(Ion.ADD_CIRCLE, size = 14.dp, tint = c.primary)
        Spacer(Modifier.width(6.dp))
        Text(stringResource(R.string.conf_chat_task_in), color = c.text2, fontSize = 12.sp)
        Spacer(Modifier.width(6.dp))
        Box {
            val board = state.boards.firstOrNull { it.id == state.boardId }
            Text(
                board?.name ?: stringResource(R.string.conf_chat_task_no_board),
                color = if (board != null) c.primary else c.text3,
                fontSize = 12.sp,
                fontWeight = FontWeight.Medium,
                modifier = Modifier.clickableNoRipple(enabled = state.boards.isNotEmpty()) { open = true },
            )
            TDropdown(expanded = open, onDismiss = { open = false }, scrollable = true) {
                state.boards.forEach { b ->
                    TMenuItem(
                        b.name,
                        onClick = {
                            onPickBoard(b.id)
                            open = false
                        },
                        trailing = { Text(b.project, color = c.text3, fontSize = 12.sp) },
                    )
                }
            }
        }
        if (title.isBlank()) {
            Spacer(Modifier.width(6.dp))
            Text(stringResource(R.string.conf_chat_task_title), color = c.text3, fontSize = 12.sp)
        }
    }
}
