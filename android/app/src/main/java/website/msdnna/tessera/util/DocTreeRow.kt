package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.Document

/** A document plus how deep it sits in the tree, ready for a flat list. */
data class DocTreeRow(val doc: Document, val depth: Int)

private const val MAX_TREE_DEPTH = 32

/**
 * Flattens the document tree (documents nest via `parent_id`) into rows in
 * reading order, depth-first, siblings ordered by the server's `position`.
 *
 * Two cases the server does not rule out and a naive walk gets wrong:
 * a document whose parent is not in the list (project-scoped listing, or a
 * parent the caller cannot see) is an **orphan** and must still be shown — at
 * the root, rather than dropped; and a parent cycle must terminate. Both are
 * handled by walking from the roots and emitting whatever the walk missed.
 */
fun documentTree(docs: List<Document>): List<DocTreeRow> {
    if (docs.isEmpty()) return emptyList()
    val byId = docs.associateBy { it.id }
    val children = docs.groupBy { it.parentId?.takeIf { id -> byId.containsKey(id) } }
    val out = mutableListOf<DocTreeRow>()
    val seen = mutableSetOf<String>()

    fun walk(parent: String?, depth: Int) {
        if (depth > MAX_TREE_DEPTH) return
        for (doc in children[parent].orEmpty().sortedWith(compareBy({ it.position }, { it.createdAt }))) {
            if (!seen.add(doc.id)) continue
            out += DocTreeRow(doc, depth)
            walk(doc.id, depth + 1)
        }
    }
    walk(null, 0)

    // Anything the walk never reached is in a parent cycle: show it flat rather
    // than lose it.
    for (doc in docs) {
        if (seen.add(doc.id)) out += DocTreeRow(doc, 0)
    }
    return out
}
