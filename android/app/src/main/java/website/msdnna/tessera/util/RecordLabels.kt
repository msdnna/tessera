package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.Member

/**
 * Tags and @-mentions of a note or a reminder (#3040, web «Входящие»).
 *
 * The server stores tags as names (a project tag spelled the project's way, or
 * the record's own label) and mentions as member ids. Here they are only shown:
 * the tags as one «#a #b» line, the mentions as the first names of the members
 * they point at — an id nobody in the roster owns any more (the person left)
 * is left out, as on the web.
 */
fun recordTagLine(tags: List<String>?): String =
    tags.orEmpty().filter { it.isNotBlank() }.joinToString(" ") { "#${it.trim()}" }

fun recordMentionNames(ids: List<String>?, members: List<Member>): List<String> =
    ids.orEmpty().distinct().mapNotNull { id ->
        members.firstOrNull { it.userId == id }?.let { m ->
            m.name.ifBlank { m.email }.trim().split(Regex("\\s+")).firstOrNull()?.takeIf { it.isNotBlank() }
        }
    }
