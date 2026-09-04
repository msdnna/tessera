package website.msdnna.tessera.util

// Outline of a document (#2733) — the Kotlin half of `frontend/src/utils/docToc.js`.
//
// Derived from the body every time it is read, never stored: a saved table of
// contents is a copy of something that changes with every keystroke.
//
// The anchor is the block id, the same one the editor stamps and the web
// outline links to, so tapping a row here lands on the same block the web
// client would scroll to.

/** One outline row. [depth] is the *relative* nesting, not the heading level. */
data class DocTocRow(
    val id: String,
    val level: Int,
    val text: String,
    val depth: Int,
)

/**
 * The document's outline, in reading order.
 *
 * Levels are normalised rather than trusted: a document that starts at h2 and
 * drops to h4 would otherwise be drawn with two empty indent steps in front of
 * every entry. What the outline shows is the structure the author wrote, so a
 * heading nests under the nearest preceding heading of a smaller level and is
 * indented one step from it, whatever the numbers say.
 *
 * Built from the flattened blocks rather than from the JSON because that is
 * what the reader already has, and because it keeps the two in agreement: a
 * heading the reader does not draw (inside a node type this client predates)
 * must not appear in an outline that promises to scroll to it.
 */
fun docOutline(blocks: List<DocBlock>): List<DocTocRow> {
    val out = mutableListOf<DocTocRow>()
    // Levels of the headings this one is nested under, innermost last.
    val stack = mutableListOf<Int>()
    for (block in blocks) {
        if (block !is DocHeading) continue
        while (stack.isNotEmpty() && stack.last() >= block.level) stack.removeAt(stack.lastIndex)
        out += DocTocRow(
            id = block.id,
            level = block.level,
            text = block.spans.joinToString("") { it.text }.trim(),
            depth = stack.size,
        )
        stack += block.level
    }
    return out
}

/** Index of the row's block in [blocks] — where the reader has to scroll to. */
fun docBlockIndex(blocks: List<DocBlock>, id: String): Int = blocks.indexOfFirst { it.id == id }
