package website.msdnna.tessera.util

import android.content.res.Resources
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.GitlabMember
import website.msdnna.tessera.data.model.Member

// Mention plumbing shared by the composer, the renderer and the mention card
// (port of the web `utils/mentions.js`). One row shape serves three places:
//   • the editor's @-popup — inserts [insert], shows [display] + [hint];
//   • RichContent's highlighting — matches on [insert] and [display];
//   • the mention card — resolves a tapped chip back to the person.

/** Roles as the mention card spells them — те же строки, что и в списке участников.
 *  An unknown role falls back to its raw value: inventing «Участник» for it would be
 *  worse than showing the code. */
private val RoleLabels = mapOf(
    "owner" to R.string.members_role_owner,
    "admin" to R.string.members_role_admin,
    "member" to R.string.members_role_member,
)

fun roleLabel(res: Resources, role: String?): String =
    RoleLabels[role]?.let(res::getString) ?: role.orEmpty()

/**
 * One @-mention candidate. [insert] is the text put after '@' in the content,
 * [display] the human name on screen — they differ on purpose: a comment is
 * pushed to GitLab verbatim, so «@Евгений Полянский» resolves to nobody there
 * and the GitLab login has to be what lands in the text.
 */
data class MentionItem(
    val insert: String,
    val display: String,
    /** Shown muted in the picker when the inserted text isn't what the row says. */
    val hint: String = "",
    val email: String = "",
    val role: String = "",
    /** The GitLab login this person is also mentioned by, if any — lets an old
     *  «@login» chip resolve to them even when [insert] has since changed. */
    val username: String? = null,
    val avatarUserId: String? = null,
    val avatarSrc: String? = null,
    val gitlab: Boolean = false,
)

/**
 * Merges the Tessera roster with the GitLab one into the shape mentions are
 * matched, inserted and carded by. A member's inserted login comes from their
 * own OAuth identity (`gl_username` on the member row) or from the GitLab
 * roster row pointing back at them; a member with no GitLab account keeps
 * inserting their name. A GitLab user already mapped to a member is folded into
 * that member rather than listed twice.
 */
fun buildMentionItems(members: List<Member>, gitlabMembers: List<GitlabMember>): List<MentionItem> {
    val glByUser = gitlabMembers.filter { !it.tesseraUserId.isNullOrBlank() }.associateBy { it.tesseraUserId!! }
    val memberIds = members.map { it.userId }.toSet()
    val tessera = members.map { m ->
        val login = m.glUsername.ifBlank { glByUser[m.userId]?.glUsername.orEmpty() }
        MentionItem(
            insert = login.ifBlank { m.name },
            display = m.name,
            hint = if (login.isNotBlank()) "@$login" else "",
            email = m.email,
            role = m.role,
            username = login.ifBlank { null },
            avatarUserId = m.userId,
        )
    }
    val gitlabOnly = gitlabMembers
        .filter { it.tesseraUserId == null || it.tesseraUserId !in memberIds }
        .map {
            MentionItem(
                insert = it.glUsername,
                display = it.glName.ifBlank { it.glUsername },
                username = it.glUsername,
                avatarSrc = it.glAvatarUrl,
                gitlab = true,
            )
        }
    return tessera + gitlabOnly
}

/**
 * Turns a rendered mention chip back into the person it names. Returns null for
 * a handle nobody in either roster owns — the caller then shows no card at all
 * rather than one that merely repeats the text that was tapped.
 */
fun resolveMention(items: List<MentionItem>, handle: String): MentionItem? {
    val key = handle.trim().removePrefix("@").trim().lowercase()
    if (key.isEmpty()) return null
    return items.firstOrNull { it.username?.lowercase() == key }
        ?: items.firstOrNull { it.insert.lowercase() == key }
        ?: items.firstOrNull { it.display.lowercase() == key }
}
