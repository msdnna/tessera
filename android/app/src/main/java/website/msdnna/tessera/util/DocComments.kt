package website.msdnna.tessera.util

import com.google.gson.JsonElement
import com.google.gson.JsonObject
import website.msdnna.tessera.data.model.DocumentComment

// Threading and anchoring for document annotations (#2730, §5 of #2894) — the
// app's copy of the web `utils/docComments.js` rules.
//
// The server stores comments flat: roots and replies in one table, one list per
// document. Everything below turns that list into what the sheet draws, and it
// is kept as pure functions for the same reason the web keeps them out of the
// panel — the interesting parts (orphans, ordering, a block that was deleted
// under a live discussion) are worth testing without a screen.

/** A root comment plus its replies, in the order the server returned them. */
data class DocThread(
    val root: DocumentComment,
    val replies: List<DocumentComment> = emptyList(),
) {
    val id: String get() = root.id
    val blockId: String get() = root.blockId
    val resolved: Boolean get() = root.isResolved
}

/**
 * Threads split by where they point. Kept apart rather than flagged inline
 * because the sheet gives each group its own heading: a remark about a paragraph
 * that no longer exists needs saying so, not hiding.
 */
data class DocThreadGroups(
    val anchored: List<DocThread> = emptyList(),
    val document: List<DocThread> = emptyList(),
    val detached: List<DocThread> = emptyList(),
) {
    val isEmpty: Boolean get() = anchored.isEmpty() && document.isEmpty() && detached.isEmpty()
}

/**
 * Groups a flat comment list into threads.
 *
 * A reply whose root is missing is dropped rather than promoted: the root
 * carries the anchor and the resolved state, so a lone reply would render as an
 * answer to a question nobody can see.
 */
fun buildDocThreads(comments: List<DocumentComment>): List<DocThread> {
    val roots = LinkedHashMap<String, MutableList<DocumentComment>>()
    val rootRows = LinkedHashMap<String, DocumentComment>()
    for (c in comments) {
        if (c.id.isNotBlank() && c.isRoot) {
            rootRows[c.id] = c
            roots[c.id] = mutableListOf()
        }
    }
    for (c in comments) {
        val parent = c.parentId ?: continue
        roots[parent]?.add(c)
    }
    return rootRows.map { (id, root) -> DocThread(root, roots[id].orEmpty()) }
}

/**
 * Orders threads the way the panel reads: unresolved first, newest first inside
 * each group.
 *
 * Unresolved first because this is a to-do list — a document with fifty settled
 * remarks would otherwise bury the one still waiting for an answer. Timestamps
 * are RFC3339 from the same server, so comparing them as text is comparing them
 * as time.
 */
fun sortDocThreads(threads: List<DocThread>): List<DocThread> =
    threads.sortedWith(
        compareBy<DocThread> { if (it.resolved) 1 else 0 }
            .thenByDescending { it.root.createdAt },
    )

/**
 * Splits threads by their anchor: a block still in the document, the document as
 * a whole, or a block that has since been deleted.
 *
 * Detached threads are kept and shown apart, never dropped — a paragraph being
 * rewritten is the normal course of a review, and deleting the discussion that
 * asked for the rewrite is not something anybody requested.
 *
 * [blockIds] is document order, not just membership: the anchored group is laid
 * out in reading order so the sheet walks the document top to bottom. Within one
 * block the order [sortDocThreads] gave stands (Kotlin's sort is stable).
 */
fun splitDocThreads(threads: List<DocThread>, blockIds: List<String>): DocThreadGroups {
    val rank = blockIds.withIndex().associate { (i, id) -> id to i }
    val anchored = mutableListOf<DocThread>()
    val document = mutableListOf<DocThread>()
    val detached = mutableListOf<DocThread>()
    for (t in threads) {
        when {
            t.blockId.isBlank() -> document += t
            rank.containsKey(t.blockId) -> anchored += t
            else -> detached += t
        }
    }
    return DocThreadGroups(
        anchored = anchored.sortedBy { rank[it.blockId] ?: 0 },
        document = document,
        detached = detached,
    )
}

/** Open (unresolved) threads — what the badge over the document counts. A
 *  settled discussion should stop asking for attention. */
fun docOpenThreadCount(threads: List<DocThread>): Int = threads.count { !it.resolved }

/** Open threads on one block, for the "N обсуждений" line of a focused sheet. */
fun docOpenThreadCount(threads: List<DocThread>, blockId: String): Int =
    threads.count { !it.resolved && it.blockId == blockId }

/**
 * Display name for a comment's author, with the same fallback chain the presence
 * badges use: the same person must not be «Иван» in the roster and an email
 * address in the thread right below it.
 *
 * [fallback] is the localised «участник» — resource lookup belongs to the caller,
 * this file has no Context.
 */
fun docCommentAuthor(comment: DocumentComment, fallback: String): String {
    val name = comment.authorName?.trim().orEmpty()
    if (name.isNotEmpty()) return name
    val local = comment.authorEmail.orEmpty().substringBefore('@').trim()
    return local.ifEmpty { fallback }
}

/**
 * Every block id in the document, in the order the blocks appear.
 *
 * Deliberately not [parseDocBlocks]`.map { it.id }`: the reader invents an id for
 * a node that has none (it needs a list key), and an invented id would make a
 * detached thread look anchored to a block that does not exist. Only real
 * `attrs.id` values anchor anything.
 */
fun docBlockIdsInOrder(content: JsonElement?): List<String> {
    val out = mutableListOf<String>()
    val seen = mutableSetOf<String>()
    walkBlockIds(content, out, seen, 0)
    return out
}

private const val MAX_ID_DEPTH = 32

private fun walkBlockIds(node: JsonElement?, out: MutableList<String>, seen: MutableSet<String>, depth: Int) {
    if (depth > MAX_ID_DEPTH) return
    val obj = node as? JsonObject ?: return
    val id = (obj.get("attrs") as? JsonObject)?.get("id")?.takeIf { it.isJsonPrimitive }?.asString
    if (!id.isNullOrBlank() && seen.add(id)) out += id
    val content = obj.get("content")?.takeIf { it.isJsonArray }?.asJsonArray ?: return
    for (child in content) walkBlockIds(child, out, seen, depth + 1)
}
