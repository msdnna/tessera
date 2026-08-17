package website.msdnna.tessera.util

import com.google.gson.JsonArray
import com.google.gson.JsonElement
import com.google.gson.JsonObject

// Read-only rendering model for a document body (#2735).
//
// The body is a ProseMirror tree (`documents.content`), the same one the web
// editor writes and `handlers/document_schema.go` validates. Android only reads
// it, so instead of a node tree this flattens to a list of rows a LazyColumn can
// emit one by one — nesting that matters for reading (list depth, table cells)
// survives as a field, nesting that does not is collapsed.
//
// Pure and Gson-only on purpose: `org.json` is a stub in unit tests, and this is
// the layer that has to be covered by them (Android tests here are util-level).

/** An inline run of text plus the marks the editor can put on it. */
data class DocSpan(
    val text: String,
    val bold: Boolean = false,
    val italic: Boolean = false,
    val underline: Boolean = false,
    val strike: Boolean = false,
    val code: Boolean = false,
    val href: String? = null,
    val color: String? = null,
)

/** One rendered row. [id] is the block anchor D4 locks and D5 annotates. */
sealed interface DocBlock {
    val id: String
}

data class DocParagraph(
    override val id: String,
    val spans: List<DocSpan>,
    val align: String? = null,
    val indent: Int = 0,
) : DocBlock

data class DocHeading(
    override val id: String,
    val level: Int,
    val spans: List<DocSpan>,
    val align: String? = null,
) : DocBlock

/**
 * A flattened list row. [marker] is what to draw in the gutter — a bullet, an
 * ordinal, or empty for task items, which draw a checkbox from [checked].
 */
data class DocListRow(
    override val id: String,
    val spans: List<DocSpan>,
    val marker: String,
    val depth: Int,
    val checked: Boolean? = null,
) : DocBlock

data class DocQuote(override val id: String, val spans: List<DocSpan>) : DocBlock

data class DocCode(override val id: String, val language: String, val text: String) : DocBlock

data class DocDivider(override val id: String) : DocBlock

data class DocImage(override val id: String, val src: String, val alt: String) : DocBlock

data class DocTableCell(val spans: List<DocSpan>, val header: Boolean)

data class DocTableRow(val cells: List<DocTableCell>)

data class DocTable(override val id: String, val rows: List<DocTableRow>) : DocBlock

private const val BULLET = "•"
private const val MAX_DEPTH = 32

/**
 * Parses a document body into rows. Unknown node types are skipped rather than
 * rejected: the schema is enforced on write by the backend, and a reader that
 * blanks a whole document over one unfamiliar node is worse than one that drops
 * the node — the phone is behind the web client, not ahead of it.
 */
fun parseDocBlocks(content: JsonElement?): List<DocBlock> {
    val root = content as? JsonObject ?: return emptyList()
    val out = mutableListOf<DocBlock>()
    val counter = intArrayOf(0)
    appendNodes(root.nodes("content"), out, counter, 0)
    return out
}

/** Plain text of a body, for previews and content descriptions. */
fun docPlainText(content: JsonElement?): String =
    parseDocBlocks(content).joinToString("\n") { block ->
        when (block) {
            is DocParagraph -> block.spans.joinToString("") { it.text }

            is DocHeading -> block.spans.joinToString("") { it.text }

            is DocListRow -> block.spans.joinToString("") { it.text }

            is DocQuote -> block.spans.joinToString("") { it.text }

            is DocCode -> block.text

            is DocImage -> block.alt

            is DocTable -> block.rows.joinToString("\n") { row ->
                row.cells.joinToString("\t") { cell -> cell.spans.joinToString("") { it.text } }
            }

            is DocDivider -> ""
        }
    }.trim()

private fun appendNodes(nodes: List<JsonElement>, out: MutableList<DocBlock>, counter: IntArray, depth: Int) {
    if (depth > MAX_DEPTH) return
    for (element in nodes) {
        val node = element as? JsonObject ?: continue
        appendNode(node, out, counter, depth)
    }
}

private fun appendNode(node: JsonObject, out: MutableList<DocBlock>, counter: IntArray, depth: Int) {
    val id = blockId(node, counter)
    when (node.str("type")) {
        "paragraph" -> out += DocParagraph(id, spansOf(node), node.attr("textAlign"), node.attrInt("indent") ?: 0)

        "heading" -> out += DocHeading(id, (node.attrInt("level") ?: 1).coerceIn(1, 6), spansOf(node), node.attr("textAlign"))

        "bulletList", "orderedList", "taskList" -> appendList(node, out, counter, depth, 0)

        "blockquote" -> appendQuote(node, out, counter, depth)

        "codeBlock" -> out += DocCode(id, node.attr("language").orEmpty(), textOf(node))

        "horizontalRule" -> out += DocDivider(id)

        "image" -> out += DocImage(id, node.attr("src").orEmpty(), node.attr("alt").orEmpty())

        "table" -> out += DocTable(id, tableRows(node))

        // Anything else (a node type this client predates) contributes its
        // children rather than vanishing with them.
        else -> appendNodes(node.nodes("content"), out, counter, depth + 1)
    }
}

/**
 * Flattens a list and its nested lists. The ordinal counter is per level and
 * restarts on each nested list, which is what `attrs.start` means in the editor.
 */
private fun appendList(list: JsonObject, out: MutableList<DocBlock>, counter: IntArray, depth: Int, listDepth: Int) {
    if (depth > MAX_DEPTH) return
    val kind = list.str("type")
    var ordinal = if (kind == "orderedList") (list.attrInt("start") ?: 1) else 1
    for (element in list.nodes("content")) {
        val item = element as? JsonObject ?: continue
        val itemId = blockId(item, counter)
        val marker = when (kind) {
            "orderedList" -> "$ordinal."
            "taskList" -> ""
            else -> BULLET
        }
        val checked = if (kind == "taskList") item.attrBool("checked") ?: false else null
        // A list item holds a paragraph plus, optionally, a nested list. The
        // first paragraph is the row's own text; everything after it is emitted
        // as its own rows so nested content is never swallowed.
        val children = item.nodes("content")
        var ownSpans: List<DocSpan> = emptyList()
        var tookOwn = false
        val rest = mutableListOf<JsonObject>()
        for (child in children) {
            val childNode = child as? JsonObject ?: continue
            if (!tookOwn && childNode.str("type") == "paragraph") {
                ownSpans = spansOf(childNode)
                tookOwn = true
            } else {
                rest += childNode
            }
        }
        out += DocListRow(itemId, ownSpans, marker, listDepth, checked)
        for (child in rest) {
            when (child.str("type")) {
                "bulletList", "orderedList", "taskList" -> appendList(child, out, counter, depth + 1, listDepth + 1)
                else -> appendNode(child, out, counter, depth + 1)
            }
        }
        ordinal++
    }
}

private fun appendQuote(node: JsonObject, out: MutableList<DocBlock>, counter: IntArray, depth: Int) {
    if (depth > MAX_DEPTH) return
    val children = node.nodes("content")
    if (children.isEmpty()) {
        out += DocQuote(blockId(node, counter), emptyList())
        return
    }
    for (element in children) {
        val child = element as? JsonObject ?: continue
        out += DocQuote(blockId(child, counter), spansOf(child))
    }
}

private fun tableRows(table: JsonObject): List<DocTableRow> =
    table.nodes("content").mapNotNull { rowElement ->
        val row = rowElement as? JsonObject ?: return@mapNotNull null
        if (row.str("type") != "tableRow") return@mapNotNull null
        val cells = row.nodes("content").mapNotNull { cellElement ->
            val cell = cellElement as? JsonObject ?: return@mapNotNull null
            val header = cell.str("type") == "tableHeader"
            if (!header && cell.str("type") != "tableCell") return@mapNotNull null
            DocTableCell(inlineSpans(cell), header)
        }
        DocTableRow(cells)
    }

/** Inline spans of a block node, descending through wrapper nodes (table cells). */
private fun inlineSpans(node: JsonObject): List<DocSpan> {
    val out = mutableListOf<DocSpan>()
    collectSpans(node.nodes("content"), out, 0)
    return merge(out)
}

private fun spansOf(node: JsonObject): List<DocSpan> = inlineSpans(node)

private fun collectSpans(nodes: List<JsonElement>, out: MutableList<DocSpan>, depth: Int) {
    if (depth > MAX_DEPTH) return
    for (element in nodes) {
        val node = element as? JsonObject ?: continue
        when (node.str("type")) {
            "text" -> node.str("text")?.takeIf { it.isNotEmpty() }?.let { out += span(it, node) }
            "hardBreak" -> out += DocSpan("\n")
            else -> collectSpans(node.nodes("content"), out, depth + 1)
        }
    }
}

private fun span(text: String, node: JsonObject): DocSpan {
    var span = DocSpan(text)
    for (element in node.nodes("marks")) {
        val mark = element as? JsonObject ?: continue
        span = when (mark.str("type")) {
            "bold" -> span.copy(bold = true)
            "italic" -> span.copy(italic = true)
            "underline" -> span.copy(underline = true)
            "strike" -> span.copy(strike = true)
            "code" -> span.copy(code = true)
            "link" -> span.copy(href = mark.attr("href"))
            "textStyle" -> span.copy(color = mark.attr("color"))
            else -> span
        }
    }
    return span
}

/** Collapses adjacent runs that carry identical marks — the editor splits text
 *  on every mark boundary, and re-joining keeps the rendered string short. */
private fun merge(spans: List<DocSpan>): List<DocSpan> {
    val out = mutableListOf<DocSpan>()
    for (span in spans) {
        val last = out.lastOrNull()
        if (last != null && last.copy(text = "") == span.copy(text = "")) {
            out[out.lastIndex] = last.copy(text = last.text + span.text)
        } else {
            out += span
        }
    }
    return out
}

private fun textOf(node: JsonObject): String {
    val out = mutableListOf<DocSpan>()
    collectSpans(node.nodes("content"), out, 0)
    return out.joinToString("") { it.text }
}

/**
 * The anchor id the editor stamps on every block. Synthesised when absent so a
 * LazyColumn key is always unique — a document written before BlockId existed
 * would otherwise collapse to a single row.
 */
private fun blockId(node: JsonObject, counter: IntArray): String {
    counter[0]++
    return node.attr("id")?.takeIf { it.isNotBlank() } ?: "b${counter[0]}"
}

private fun JsonObject.str(key: String): String? =
    get(key)?.takeIf { it.isJsonPrimitive }?.asString

/** Child nodes under [key]. A JsonArray is not a Kotlin collection, so it is
 *  copied out once here rather than iterated as one at every call site. */
private fun JsonObject.nodes(key: String): List<JsonElement> {
    val array = get(key) as? JsonArray ?: return emptyList()
    val out = ArrayList<JsonElement>(array.size())
    for (i in 0 until array.size()) out += array[i]
    return out
}

private fun JsonObject.attrs(): JsonObject? = get("attrs") as? JsonObject

private fun JsonObject.attr(key: String): String? = attrs()?.str(key)

private fun JsonObject.attrInt(key: String): Int? =
    attrs()?.get(key)?.takeIf { it.isJsonPrimitive }?.let { runCatching { it.asInt }.getOrNull() }

private fun JsonObject.attrBool(key: String): Boolean? =
    attrs()?.get(key)?.takeIf { it.isJsonPrimitive }?.let { runCatching { it.asBoolean }.getOrNull() }
