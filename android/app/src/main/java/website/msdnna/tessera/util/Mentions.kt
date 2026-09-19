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
 *
 * "Pointing back at them" means *either* link the server reports: a GitLab row
 * carries the Tessera user in `tessera_user_id` when that person signed in
 * through GitLab, and in `tessera_user_id_pat` when they connected a personal
 * access token instead. Reading only the first one left PAT-linked people
 * inserting their own name — which notifies them in Tessera but resolves to
 * nobody once the comment is pushed to GitLab — while their roster row was
 * listed a second time as an outsider, whose handle notifies nobody at all.
 */
fun buildMentionItems(members: List<Member>, gitlabMembers: List<GitlabMember>): List<MentionItem> {
    fun GitlabMember.linkedUserId(): String? =
        tesseraUserId?.takeIf { it.isNotBlank() } ?: tesseraUserIdPat?.takeIf { it.isNotBlank() }
    val glByUser = gitlabMembers.mapNotNull { g -> g.linkedUserId()?.let { it to g } }.toMap()
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
        .filter { it.linkedUserId().let { uid -> uid == null || uid !in memberIds } }
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
 * Markup that disappears into a tag, leaving the '@' at the start of a text node:
 * emphasis/strike delimiters, a link label's bracket, an escape. The renderer sees
 * `<em>@Иван`, i.e. the '>' its own boundary class allows.
 */
private const val MARKUP_LEAD = "*_~[\\"

/**
 * The workspace members a piece of content mentions, in the order they are written
 * and without repeats — the list handed to the server so those people are notified.
 *
 * The promise being kept is that **the chip the reader sees is the person told**.
 * Scanning the source for `"@" + name` broke it in both directions: «@Иван» inside
 * a fenced block or `mail@root` in a sentence drew no chip yet notified, while a
 * member mentioned by the GitLab login the picker inserts on their behalf (the
 * login can come from the GitLab roster rather than their own member row) drew a
 * resolved chip yet notified nobody.
 *
 * So this reproduces what `highlightMentions` in `RichContent.kt` draws: code is
 * masked out ([codeMasked]), the '@' has to open a token (start of text,
 * whitespace, '(' — or markup that becomes a tag), and the longest handle wins.
 * There is no Kotlin markdown parser here (the app's copy is JS inside the
 * WebView) and a comment is posted without one, so the rules are walked instead,
 * pinned by a table measured off the renderer in `MentionsTest`.
 *
 * Rows with no [MentionItem.avatarUserId] — GitLab-only users — drop out on their
 * own: the handle stays in the text for GitLab to resolve on write-back, and
 * Tessera has nobody to notify.
 */
fun mentionIdsIn(src: String, items: List<MentionItem>): List<String> {
    if (src.isBlank() || items.isEmpty()) return emptyList()
    val text = codeMasked(src)
    val handles = items.flatMap { listOf(it.insert, it.display) }
        .filter { it.isNotBlank() }
        .distinct()
        .sortedByDescending { it.length }
    val out = LinkedHashSet<String>()
    var i = 0
    while (i < text.length) {
        val at = text.indexOf('@', i)
        if (at < 0) break
        val handle = if (opensMention(text, at)) handles.firstOrNull { text.startsWith(it, at + 1) } else null
        handle?.let { resolveMention(items, it)?.avatarUserId?.let(out::add) }
        i = at + 1 + (handle?.length ?: 0)
    }
    return out.toList()
}

/**
 * Whom an edit ADDED: the members [after] mentions, minus those [before] already
 * did — the list `PATCH /comments/:id` is handed.
 *
 * Only the difference travels, and it has to be computed here. The server keeps
 * the previous body as plain text, without the roster a handle becomes a chip
 * against, so it can tell neither who was mentioned before nor who is now;
 * sending the whole set instead would ping everyone named in the comment again
 * every time its author fixed a typo. Sending nothing — what the edit path did
 * until #2932 — breaks the promise the chip makes: the edit composer offers the
 * same @-popup the main one does, so «@Иван, посмотри» added by an edit looks
 * exactly like a mention Иван was told about.
 */
fun addedMentionIds(before: String, after: String, items: List<MentionItem>): List<String> {
    val was = mentionIdsIn(before, items).toSet()
    return mentionIdsIn(after, items).filterNot(was::contains)
}

/** True when the '@' at [at] starts a token the renderer would chip. */
private fun opensMention(text: String, at: Int): Boolean {
    var i = at - 1
    // A delimiter run only opens markup when itself preceded by a boundary:
    // `_@Иван_` is emphasis, `mail_@root` is one plain word.
    while (i >= 0 && text[i] in MARKUP_LEAD) i--
    if (i < 0) return true
    val c = text[i]
    // '>' and '|' close a block or table cell — the '@' again opens its text.
    return c.isWhitespace() || c == '(' || c == '>' || c == '|'
}

/** An open `@`-query in the composer: [start] is the index of the '@', [query] what follows it. */
data class MentionQuery(val start: Int, val query: String)

// Unlike the `/`-trigger a mention may open anywhere after whitespace, so this
// one is not anchored to the start of a line — see [detectSlashQuery].
//
// `\z`, not `$`: java's `$` also matches *before* a trailing newline, so a handle
// the user has already finished with Enter would keep the popup open — and
// [MentionQuery.start], measured back from the caret, would land on the newline
// instead of the '@'. JS has no such rule, which is why the web regex in
// `MarkdownEditor.vue` can end in `$`.
private val MENTION_RE = Regex("(^|\\s)@([^\\s@]*)\\z")

/** Finds an open `@`-query in the text up to the caret, or null. */
fun detectMentionQuery(upto: String?): MentionQuery? {
    val text = upto ?: return null
    val m = MENTION_RE.find(text) ?: return null
    val query = m.groupValues[2]
    return MentionQuery(start = text.length - query.length - 1, query = query)
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
