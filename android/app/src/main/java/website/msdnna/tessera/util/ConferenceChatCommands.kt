package website.msdnna.tessera.util

//
// The call chat's «/» and «@» (#3055, web `utils/confChatCommands.js` — same
// rules, kept in step).
//
// «/задача Название» creates a task on a board of the workspace and leaves the
// line «создал(а) задачу #N» in the chat (`POST /conferences/:id/tasks`). «@Имя»
// names a member: the line draws a chip, and the person is notified — the ids
// travel with the message, as for a comment.

/** A member as the chat matches them. */
data class ChatMember(val userId: String, val name: String, val email: String = "")

/** The command's names: Russian, English and their one-letter shorthands. */
val CONF_TASK_COMMANDS = listOf("задача", "task", "з", "t")

/** The open «/» query while the draft is still the command's name, or null. */
fun chatSlashQuery(draft: String): String? = Regex("^/(\\S*)$").find(draft)?.groupValues?.get(1)

/** Whether a «/» query still matches the task command. */
fun chatCommandMatches(query: String): Boolean = CONF_TASK_COMMANDS.any { it.startsWith(query.lowercase()) }

/**
 * The task a draft asks for: the title (possibly blank — the bare command)
 * when the draft is «/задача …», null for an ordinary message.
 */
fun chatTaskCommand(draft: String): String? {
    val m = Regex("^/(\\S+)(?:\\s+([\\s\\S]*))?$").find(draft.trim()) ?: return null
    if (m.groupValues[1].lowercase() !in CONF_TASK_COMMANDS) return null
    return m.groupValues[2].replace(Regex("\\s+"), " ").trim()
}

/** The open «@» query at the end of [upto]: (start index, query), or null. */
fun chatMentionQuery(upto: String): Pair<Int, String>? {
    val m = Regex("(^|[\\s(\\[{«\"'])@([^\\s@]*)$").find(upto) ?: return null
    val q = m.groupValues[2]
    return (upto.length - q.length - 1) to q
}

/** Members a «@» query offers: by the start of any word of the name, or the e-mail. */
fun chatMentionMatches(members: List<ChatMember>, query: String, me: String = "", limit: Int = 8): List<ChatMember> {
    val q = query.lowercase()
    return members.filter { m ->
        m.userId.isNotBlank() && m.userId != me && (
            q.isEmpty() ||
                m.name.lowercase().split(Regex("\\s+")).any { it.startsWith(q) } ||
                m.name.lowercase().startsWith(q) ||
                m.email.lowercase().startsWith(q)
            )
    }.take(limit)
}

/** One piece of a line: text, or a chip naming [member]. */
data class ChatPart(val text: String, val member: ChatMember? = null)

/** The pieces of a message — every whole «@Name» of a member becomes a chip, the longest name first. */
fun chatSplitMentions(body: String, members: List<ChatMember>): List<ChatPart> {
    val roster = members.filter { it.userId.isNotBlank() && it.name.isNotBlank() }.sortedByDescending { it.name.length }
    if (roster.isEmpty() || '@' !in body) return listOf(ChatPart(body))
    val out = mutableListOf<ChatPart>()
    val buf = StringBuilder()
    var i = 0
    while (i < body.length) {
        val opens = body[i] == '@' && (i == 0 || body[i - 1].isWhitespace() || body[i - 1] in "([{«\"'")
        if (opens) {
            val rest = body.substring(i + 1)
            val m = roster.firstOrNull { r ->
                rest.startsWith(r.name, ignoreCase = true) &&
                    rest.getOrNull(r.name.length)?.let { it.isLetterOrDigit() || it == '_' } != true
            }
            if (m != null) {
                if (buf.isNotEmpty()) out += ChatPart(buf.toString())
                buf.clear()
                out += ChatPart("@" + rest.substring(0, m.name.length), m)
                i += 1 + m.name.length
                continue
            }
        }
        buf.append(body[i])
        i++
    }
    if (buf.isNotEmpty()) out += ChatPart(buf.toString())
    return out
}

/** The ids a message mentions, in order, once each — read off the same chips the line shows. */
fun chatMentionIds(body: String, members: List<ChatMember>): List<String> =
    chatSplitMentions(body, members).mapNotNull { it.member?.userId }.distinct()

/** The board a chat task lands on: the call's own, else the first. */
fun chatDefaultBoard(boardIds: List<String>, preferred: String?): String? =
    preferred?.takeIf { it in boardIds } ?: boardIds.firstOrNull()
