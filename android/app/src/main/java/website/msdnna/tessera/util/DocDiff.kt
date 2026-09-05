package website.msdnna.tessera.util

import com.google.gson.JsonArray
import com.google.gson.JsonElement
import com.google.gson.JsonObject
import website.msdnna.tessera.data.model.DocumentVersion

// Block-level comparison of two document versions (#2731, §6 of #2894) — the
// app's copy of the web `utils/docDiff.js` rules.
//
// The diff is anchored on block ids, the stable ids the editor stamps on every
// top-level node, and not on text positions: a paragraph dragged three places up
// is *moved*, not "deleted here and added there", and one edited word inside a
// long paragraph does not repaint the paragraphs around it. Blocks without an id
// (imported content, or documents written before the ids existed) fall back to
// matching by position — the best available answer when there is nothing stable
// to match on.

/** Status of a block in the comparison. */
enum class DocDiffStatus { SAME, ADDED, REMOVED, CHANGED, MOVED }

/**
 * One top-level block of a document, as the comparison sees it.
 *
 * [node] is kept whole rather than as its flattened text: a heading that became
 * level 3, a link that now points elsewhere and a changed alignment are all
 * edits the plain text does not show. Comparing the parsed trees (Gson compares
 * objects structurally) rather than their serialised form also means a server
 * that reorders two JSON keys does not read as an edit.
 */
data class DocDiffBlock(
    val id: String,
    val type: String,
    val text: String,
    val node: JsonElement?,
)

/** A row of the comparison: one block with what happened to it. [prevText] is
 *  the wording an edited block had before — «изменено» without the old text asks
 *  the reader to remember what they opened the comparison to look up. */
data class DocDiffRow(
    val status: DocDiffStatus,
    val type: String,
    val text: String,
    val prevText: String = "",
)

/** What changed, for the one line above the comparison. */
data class DocDiffSummary(
    val added: Int = 0,
    val removed: Int = 0,
    val changed: Int = 0,
    val moved: Int = 0,
    val identical: Boolean = true,
)

private const val MAX_DIFF_DEPTH = 32

/**
 * Flattens one node into the text the comparison shows.
 *
 * Nested structure (list items, table cells) is joined with spaces rather than
 * walked separately: the unit of comparison is the block, and a list whose third
 * item changed is a changed list. Splitting it further would put anchors below
 * the level the ids are stamped at — exactly the guesswork the id-based diff
 * exists to avoid.
 *
 * [imageLabel] is the localised «изображение» for a picture with neither alt nor
 * src; resource lookup belongs to the caller, this file has no Context.
 */
fun docDiffText(node: JsonElement?, imageLabel: String = ""): String =
    flattenDiffText(node, imageLabel, 0).replace(WHITESPACE, " ").trim()

private val WHITESPACE = Regex("\\s+")

private fun flattenDiffText(node: JsonElement?, imageLabel: String, depth: Int): String {
    if (depth > MAX_DIFF_DEPTH) return ""
    val obj = node as? JsonObject ?: return ""
    val attrs = obj.get("attrs") as? JsonObject
    return when (obj.str("type")) {
        "text" -> obj.str("text")

        "image" -> attrs.str("alt").ifBlank { attrs.str("src") }.ifBlank { imageLabel }

        "horizontalRule" -> "———"

        else -> (obj.get("content") as? JsonArray)
            ?.joinToString(" ") { flattenDiffText(it, imageLabel, depth + 1) }
            ?.replace(WHITESPACE, " ")
            ?.trim()
            .orEmpty()
    }
}

private fun JsonObject?.str(field: String): String {
    val value = this?.get(field) ?: return ""
    return if (value.isJsonPrimitive) value.asString else ""
}

/** The top-level blocks of a document, in order. */
fun docDiffBlocks(doc: JsonElement?, imageLabel: String = ""): List<DocDiffBlock> {
    val nodes = (doc as? JsonObject)?.get("content") as? JsonArray ?: return emptyList()
    return nodes.map { node ->
        val obj = node as? JsonObject
        DocDiffBlock(
            id = (obj?.get("attrs") as? JsonObject).str("id"),
            type = obj.str("type").ifBlank { "paragraph" },
            text = docDiffText(node, imageLabel),
            node = node,
        )
    }
}

/** A row being assembled. [src] is the block on the *old* side it came from —
 *  bookkeeping for placing the deletions by identity rather than by text, since
 *  two identical paragraphs are ordinary and matching on their words would file
 *  a deletion next to the wrong one. */
private class DiffRowDraft(
    val status: DocDiffStatus,
    val type: String,
    val text: String,
    val prevText: String = "",
    val src: DocDiffBlock? = null,
)

/**
 * Compares two document versions block by block.
 *
 * The result is a single ordered list rather than two columns: the panel renders
 * it as one readable document with removals struck through in place, which is
 * what answers "что изменилось здесь" without the reader diffing the diff.
 * Removed blocks are placed after the block they used to follow, so a deletion
 * stays where it happened.
 */
fun diffDocs(oldDoc: JsonElement?, newDoc: JsonElement?, imageLabel: String = ""): List<DocDiffRow> {
    val before = docDiffBlocks(oldDoc, imageLabel)
    val after = docDiffBlocks(newDoc, imageLabel)

    // Ids are unique per document, but a copy-paste can duplicate one; keeping
    // the first occurrence makes the pairing deterministic instead of
    // order-dependent.
    val beforeById = LinkedHashMap<String, DocDiffBlock>()
    before.forEach { if (it.id.isNotBlank()) beforeById.putIfAbsent(it.id, it) }
    // Blocks with no id are paired positionally among themselves — see the file
    // comment for why this fallback exists at all.
    val anonBefore = before.filter { it.id.isBlank() }
    val anonAfter = after.filter { it.id.isBlank() }
    val anonPairs = HashMap<DocDiffBlock, DocDiffBlock>()
    anonAfter.forEachIndexed { i, block -> anonBefore.getOrNull(i)?.let { anonPairs[block] = it } }

    // Which blocks survived, on both sides and by the same rule. The web filters
    // the two sides differently (its old-side test never matches an anonymous
    // block), and in a document that mixes id'd and imported blocks that
    // asymmetry shifts the indices and reports untouched paragraphs as moved.
    val survivedAfter = after.filter { block ->
        if (block.id.isBlank()) anonPairs.containsKey(block) else beforeById.containsKey(block.id)
    }
    val pairedBefore = survivedAfter.mapNotNull { pairFor(it, beforeById, anonPairs) }
    val survivedBefore = before.filter { block -> pairedBefore.any { it === block } }

    val matched = ArrayList<DocDiffBlock>()
    val rows = ArrayList<DiffRowDraft>()

    for (block in after) {
        val prev = pairFor(block, beforeById, anonPairs)
        if (prev == null) {
            rows += DiffRowDraft(DocDiffStatus.ADDED, block.type, block.text)
        } else {
            matched += prev
            rows += if (prev.node != block.node) {
                DiffRowDraft(DocDiffStatus.CHANGED, block.type, block.text, prevText = prev.text, src = prev)
            } else {
                // Same content, different neighbourhood: the block was dragged.
                // Position is compared among *surviving* blocks only —
                // otherwise deleting a paragraph would report everything below
                // it as moved.
                val prevOrder = survivedBefore.indexOfFirst { it === prev }
                val nextOrder = survivedAfter.indexOfFirst { it === block }
                val moved = prevOrder != -1 && prevOrder != nextOrder
                DiffRowDraft(
                    if (moved) DocDiffStatus.MOVED else DocDiffStatus.SAME,
                    block.type,
                    block.text,
                    src = prev,
                )
            }
        }
    }

    // Deletions, put back where they happened: right after the last surviving
    // block that preceded them. Walking `before` forwards keeps several
    // deletions in their original order relative to each other.
    for (i in before.indices) {
        val block = before[i]
        if (matched.any { it === block }) continue
        val anchor = (i - 1 downTo 0).map { before[it] }.firstOrNull { candidate -> matched.any { it === candidate } }
        var at = if (anchor == null) 0 else rows.indexOfFirst { it.src === anchor } + 1
        // Skip past deletions already filed under the same anchor.
        while (rows.getOrNull(at)?.status == DocDiffStatus.REMOVED) at++
        rows.add(at, DiffRowDraft(DocDiffStatus.REMOVED, block.type, block.text))
    }

    return rows.map { DocDiffRow(it.status, it.type, it.text, it.prevText) }
}

private fun pairFor(
    block: DocDiffBlock,
    beforeById: Map<String, DocDiffBlock>,
    anonPairs: Map<DocDiffBlock, DocDiffBlock>,
): DocDiffBlock? = if (block.id.isBlank()) anonPairs[block] else beforeById[block.id]

/** Counts what changed. An empty comparison is [DocDiffSummary.identical] — two
 *  empty documents differ in nothing. */
fun docDiffSummary(rows: List<DocDiffRow>): DocDiffSummary = DocDiffSummary(
    added = rows.count { it.status == DocDiffStatus.ADDED },
    removed = rows.count { it.status == DocDiffStatus.REMOVED },
    changed = rows.count { it.status == DocDiffStatus.CHANGED },
    moved = rows.count { it.status == DocDiffStatus.MOVED },
    identical = rows.all { it.status == DocDiffStatus.SAME },
)

/**
 * The comparison the journal shows: [selected] against [baseline], with bodies
 * taken from the cache the panel fills.
 *
 * Older on the left whichever way round the two were picked: the diff reads
 * "what changed to get from that version to this one", and a journal is walked
 * backwards in time. An empty list means "not both bodies in hand yet" — the
 * caller shows its loading state rather than an empty "no changes", which would
 * read as an answer.
 */
fun docVersionDiff(
    selected: DocumentVersion?,
    baseline: DocumentVersion?,
    bodies: Map<String, JsonElement?>,
    imageLabel: String = "",
): List<DocDiffRow> {
    if (selected == null || baseline == null) return emptyList()
    if (!bodies.containsKey(selected.id) || !bodies.containsKey(baseline.id)) return emptyList()
    val older = if (selected.revision <= baseline.revision) selected else baseline
    val newer = if (older === selected) baseline else selected
    return diffDocs(bodies[older.id], bodies[newer.id], imageLabel)
}
