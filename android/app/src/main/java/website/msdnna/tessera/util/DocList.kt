package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.Document

/** One step of the breadcrumb trail — a container the grid was drilled into. */
data class DocCrumb(val id: String, val title: String)

/**
 * The tiles of one level of the documents grid: the children of [parentId], or
 * the roots when it is null. Siblings keep the server's order (`position`, then
 * creation time), the same order the web grid shows.
 *
 * The list arrives flat (one shape serves web, Android and MCP), so nesting is
 * rebuilt here. Two cases the API does not rule out would otherwise make a
 * document unreachable — and a document that exists but cannot be reached from
 * any level is worse than one shown in the wrong place:
 *
 * - an **orphan**, whose parent is missing from the list (a project-scoped
 *   listing, or a parent the caller cannot see), is shown at the root;
 * - a **cycle** in `parent_id` — which the API rejects but a stale client can
 *   still hold — is broken by treating everything in it as a root.
 */
fun docTiles(docs: List<Document>, parentId: String?): List<Document> {
    if (docs.isEmpty()) return emptyList()
    val byId = docs.associateBy { it.id }
    return docs.filter { effectiveParent(it, byId) == parentId }
        .sortedWith(compareBy({ it.position }, { it.createdAt }))
}

/** How many documents sit directly under [id] — the tile's «вложенных» badge. */
fun docChildCount(docs: List<Document>, id: String): Int {
    if (docs.isEmpty()) return 0
    val byId = docs.associateBy { it.id }
    return docs.count { effectiveParent(it, byId) == id }
}

/**
 * Drops trail steps whose document is gone — deleted here, or by someone else
 * between two loads. Without this the grid would sit on a level that no longer
 * exists and show nothing, with no way back but the root crumb.
 *
 * Everything below a missing step goes too: its own parent link ran through the
 * step that vanished, so keeping it would claim a nesting that is no longer true.
 */
fun pruneCrumbs(docs: List<Document>, trail: List<DocCrumb>): List<DocCrumb> {
    val byId = docs.associateBy { it.id }
    val kept = mutableListOf<DocCrumb>()
    for (crumb in trail) {
        val doc = byId[crumb.id] ?: break
        // Titles are re-read rather than trusted: a rename elsewhere in the trail
        // must not leave the breadcrumb naming a document by its old title.
        kept += DocCrumb(doc.id, doc.title)
    }
    return kept
}

/**
 * The parent a document is actually filed under: its own, unless that parent is
 * absent from the list or the chain above it loops, in which case the document
 * belongs to the root.
 */
private fun effectiveParent(doc: Document, byId: Map<String, Document>): String? {
    val parentId = doc.parentId?.takeIf { byId.containsKey(it) } ?: return null
    val seen = mutableSetOf(doc.id)
    var cur: Document? = byId[parentId]
    while (cur != null) {
        if (!seen.add(cur.id)) return null
        cur = cur.parentId?.let { byId[it] }
    }
    return parentId
}
