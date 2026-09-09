package website.msdnna.tessera.util

import com.google.gson.JsonArray
import com.google.gson.JsonObject
import com.google.gson.JsonPrimitive
import kotlin.random.Random

// Markdown → ProseMirror document JSON (#2734, §8 of #2894).
//
// The web does this through a detached TipTap editor, so its schema *is* the
// allow-list. A phone has no editor to borrow — the one it uses lives in a
// WebView on a document that already exists — so this is the one place where a
// second writer of document JSON exists, and it is kept deliberately small:
//
//   - it is only ever fed text *we* wrote (the built-in templates) or a `.md`
//     file the user picked, never the output of the office converter. Converted
//     HTML goes to the web editor, which has the full schema (see
//     [DocumentEditor]'s import handoff) — a second HTML→blocks walk in Kotlin
//     is exactly the drift #2755 spent a task fixing;
//   - anything it cannot represent degrades to a paragraph rather than being
//     dropped, because the alternative on an import is silently losing a line.
//
// The node names and the `attrs.id` convention come from `docSchema.js` and
// `docExtensions/blockId.js`; [parseDocBlocks] on the reading side is the other
// half of the same contract.

/** Block types that carry a stable id — `ID_BEARING_TYPES` in blockId.js. */
private val ID_BEARING = setOf(
    "paragraph",
    "heading",
    "blockquote",
    "codeBlock",
    "bulletList",
    "orderedList",
    "taskList",
    "horizontalRule",
    "image",
    "table",
    "pdfEmbed",
)

/**
 * A block id. Sixteen hex characters, like `newBlockId()` on the web — the id is
 * stored on every block of the document, and a 36-byte UUID per paragraph adds
 * up over a long page.
 */
fun newDocBlockId(random: Random = Random.Default): String =
    buildString(16) { repeat(8) { append("%02x".format(random.nextInt(256))) } }

/**
 * Converts Markdown into a document body.
 *
 * [random] is a seam for the tests: block ids are the one part of the output
 * that cannot be asserted on otherwise.
 */
fun markdownToDocJson(markdown: String, random: Random = Random.Default): JsonObject {
    val lines = markdown.replace("\r\n", "\n").replace('\r', '\n').split("\n")
    val body = JsonArray()
    var i = 0
    while (i < lines.size) {
        val line = lines[i]
        when {
            line.isBlank() -> i++

            fenceOf(line) != null -> {
                val fence = fenceOf(line)!!
                val language = line.trim().removePrefix(fence).trim()
                val text = StringBuilder()
                i++
                while (i < lines.size && fenceOf(lines[i]) != fence) {
                    if (text.isNotEmpty()) text.append('\n')
                    text.append(lines[i])
                    i++
                }
                // An unterminated fence still ends the document; refusing to
                // close it would swallow the rest of the file.
                if (i < lines.size) i++
                body.add(codeBlock(language, text.toString(), random))
            }

            isRule(line) -> {
                body.add(node("horizontalRule", random))
                i++
            }

            HEADING.matches(line) -> {
                val m = HEADING.find(line)!!
                val level = m.groupValues[1].length
                val heading = node("heading", random)
                heading.add("attrs", attrsWith(heading, "level", JsonPrimitive(level)))
                heading.add("content", inlineContent(m.groupValues[2].trim()))
                body.add(heading)
                i++
            }

            line.trimStart().startsWith(">") -> {
                val quoted = mutableListOf<String>()
                while (i < lines.size && lines[i].trimStart().startsWith(">")) {
                    quoted += lines[i].trimStart().removePrefix(">").removePrefix(" ")
                    i++
                }
                val quote = node("blockquote", random)
                quote.add("content", paragraphsOf(quoted, random))
                body.add(quote)
            }

            isTableRow(line) && i + 1 < lines.size && isTableDivider(lines[i + 1]) -> {
                val rows = mutableListOf(lines[i])
                i += 2
                while (i < lines.size && isTableRow(lines[i])) {
                    rows += lines[i]
                    i++
                }
                body.add(table(rows, random))
            }

            listMarker(line) != null -> {
                val kind = listMarker(line)!!
                val items = mutableListOf<String>()
                while (i < lines.size && listMarker(lines[i]) == kind) {
                    items += lines[i].trimStart().replaceFirst(markerRegex(kind), "")
                    i++
                }
                body.add(list(kind, items, random))
            }

            else -> {
                // A paragraph runs until a blank line or the start of another
                // block — the lazy-continuation rule of Markdown, which is what
                // makes wrapped prose in a file come out as one paragraph.
                val text = StringBuilder(line.trim())
                i++
                while (i < lines.size && lines[i].isNotBlank() && startsPlainLine(lines[i])) {
                    text.append(' ').append(lines[i].trim())
                    i++
                }
                body.add(paragraph(text.toString(), random))
            }
        }
    }
    // An empty file still has to be a document the editor can open, and an empty
    // `doc` with no content is not one.
    if (body.isEmpty()) body.add(paragraph("", random))
    val doc = JsonObject()
    doc.addProperty("type", "doc")
    doc.add("content", body)
    return doc
}

/**
 * The first Markdown heading of a file — what an imported document is named
 * when the file itself does not say (mirrors `firstHeading` on the web).
 */
fun firstMarkdownHeading(markdown: String): String =
    markdown.replace("\r\n", "\n").split("\n")
        .firstOrNull { HEADING_NAMED.matches(it) }
        ?.replace(HEADING_PREFIX, "")
        ?.trim()
        .orEmpty()

// ── Blocks ────────────────────────────────────────────────────────────────────

private fun node(type: String, random: Random): JsonObject {
    val obj = JsonObject()
    obj.addProperty("type", type)
    if (type in ID_BEARING) {
        val attrs = JsonObject()
        attrs.addProperty("id", newDocBlockId(random))
        obj.add("attrs", attrs)
    }
    return obj
}

/** Adds one attribute to a node that already has (or has not) an `attrs` bag. */
private fun attrsWith(owner: JsonObject, key: String, value: JsonPrimitive): JsonObject {
    val attrs = owner.getAsJsonObject("attrs") ?: JsonObject()
    attrs.add(key, value)
    return attrs
}

private fun paragraph(text: String, random: Random): JsonObject {
    val p = node("paragraph", random)
    val content = inlineContent(text)
    // ProseMirror writes an empty paragraph as a node with no `content` at all,
    // not as one with an empty array — and the schema validates against that.
    if (content.size() > 0) p.add("content", content)
    return p
}

private fun paragraphsOf(lines: List<String>, random: Random): JsonArray {
    val out = JsonArray()
    val buffer = mutableListOf<String>()
    fun flush() {
        if (buffer.isEmpty()) return
        out.add(paragraph(buffer.joinToString(" ") { it.trim() }, random))
        buffer.clear()
    }
    lines.forEach { line -> if (line.isBlank()) flush() else buffer += line }
    flush()
    if (out.size() == 0) out.add(paragraph("", random))
    return out
}

private fun codeBlock(language: String, text: String, random: Random): JsonObject {
    val block = node("codeBlock", random)
    block.add("attrs", attrsWith(block, "language", JsonPrimitive(language)))
    if (text.isNotEmpty()) {
        val content = JsonArray()
        content.add(textNode(text, DocMarks()))
        block.add("content", content)
    }
    return block
}

private enum class ListKind { BULLET, ORDERED, TASK }

private fun list(kind: ListKind, items: List<String>, random: Random): JsonObject {
    val type = when (kind) {
        ListKind.BULLET -> "bulletList"
        ListKind.ORDERED -> "orderedList"
        ListKind.TASK -> "taskList"
    }
    val list = node(type, random)
    val content = JsonArray()
    items.forEach { raw ->
        val item = JsonObject()
        item.addProperty("type", if (kind == ListKind.TASK) "taskItem" else "listItem")
        var text = raw
        if (kind == ListKind.TASK) {
            val checked = TASK_BOX.find(raw)
            val attrs = JsonObject()
            attrs.addProperty("checked", checked?.groupValues?.get(1)?.lowercase() == "x")
            item.add("attrs", attrs)
            text = raw.replaceFirst(TASK_BOX, "")
        }
        val body = JsonArray()
        body.add(paragraph(text.trim(), random))
        item.add("content", body)
        content.add(item)
    }
    list.add("content", content)
    return list
}

private fun table(rows: List<String>, random: Random): JsonObject {
    val table = node("table", random)
    val content = JsonArray()
    rows.forEachIndexed { index, raw ->
        val row = JsonObject()
        row.addProperty("type", "tableRow")
        val cells = JsonArray()
        splitTableRow(raw).forEach { cell ->
            val cellNode = JsonObject()
            // The first row of a Markdown table is its header by definition —
            // that is what the `|---|` line under it means.
            cellNode.addProperty("type", if (index == 0) "tableHeader" else "tableCell")
            val body = JsonArray()
            body.add(paragraph(cell.trim(), random))
            cellNode.add("content", body)
            cells.add(cellNode)
        }
        row.add("content", cells)
        content.add(row)
    }
    table.add("content", content)
    return table
}

// ── Inline ────────────────────────────────────────────────────────────────────

/** The marks a run of text carries. Mirrors the mark names of `docSchema.js`. */
private data class DocMarks(
    val bold: Boolean = false,
    val italic: Boolean = false,
    val code: Boolean = false,
    val strike: Boolean = false,
    val href: String = "",
)

private fun textNode(text: String, marks: DocMarks): JsonObject {
    val node = JsonObject()
    node.addProperty("type", "text")
    node.addProperty("text", text)
    val list = JsonArray()
    fun mark(type: String, attrs: JsonObject? = null) {
        val m = JsonObject()
        m.addProperty("type", type)
        attrs?.let { m.add("attrs", it) }
        list.add(m)
    }
    if (marks.bold) mark("bold")
    if (marks.italic) mark("italic")
    if (marks.strike) mark("strike")
    if (marks.code) mark("code")
    if (marks.href.isNotEmpty()) {
        val attrs = JsonObject()
        attrs.addProperty("href", marks.href)
        mark("link", attrs)
    }
    if (list.size() > 0) node.add("marks", list)
    return node
}

/**
 * Splits a line into marked runs.
 *
 * Code spans win over everything else, exactly as in Markdown: `**` inside
 * backticks is two asterisks, not the start of a bold run.
 */
private fun inlineContent(text: String): JsonArray {
    val out = JsonArray()
    if (text.isEmpty()) return out
    val buffer = StringBuilder()
    var i = 0

    fun flush(marks: DocMarks = DocMarks()) {
        if (buffer.isEmpty()) return
        out.add(textNode(buffer.toString(), marks))
        buffer.clear()
    }

    while (i < text.length) {
        val rest = text.substring(i)
        val code = CODE_SPAN.matchAt(rest, 0)
        val link = LINK.matchAt(rest, 0)
        val bold = BOLD.matchAt(rest, 0)
        val strike = STRIKE.matchAt(rest, 0)
        val italic = ITALIC.matchAt(rest, 0)
        when {
            text[i] == '\\' && i + 1 < text.length -> {
                // A backslash escapes the punctuation after it — without this a
                // literal `**` in a template body would open a bold run.
                buffer.append(text[i + 1])
                i += 2
            }

            code != null -> {
                flush()
                out.add(textNode(code.groupValues[1], DocMarks(code = true)))
                i += code.value.length
            }

            link != null -> {
                flush()
                out.add(textNode(link.groupValues[1], DocMarks(href = link.groupValues[2])))
                i += link.value.length
            }

            bold != null -> {
                flush()
                out.add(textNode(bold.groupValues[1], DocMarks(bold = true)))
                i += bold.value.length
            }

            strike != null -> {
                flush()
                out.add(textNode(strike.groupValues[1], DocMarks(strike = true)))
                i += strike.value.length
            }

            italic != null -> {
                flush()
                out.add(textNode(italic.groupValues[1], DocMarks(italic = true)))
                i += italic.value.length
            }

            else -> {
                buffer.append(text[i])
                i++
            }
        }
    }
    flush()
    return out
}

// ── Line classification ───────────────────────────────────────────────────────

private val HEADING = Regex("^\\s{0,3}(#{1,6})\\s+(.*)$")
private val HEADING_NAMED = Regex("^\\s{0,3}#{1,6}\\s+\\S.*$")
private val HEADING_PREFIX = Regex("^\\s{0,3}#{1,6}\\s+")
private val TASK_BOX = Regex("^\\s*\\[([ xX])]\\s*")
private val BULLET_MARKER = Regex("^[-*+]\\s+")
private val ORDERED_MARKER = Regex("^\\d{1,9}[.)]\\s+")
private val CODE_SPAN = Regex("`([^`]+)`")
private val LINK = Regex("\\[([^\\]]*)]\\(([^)\\s]+)\\)")
private val BOLD = Regex("\\*\\*(.+?)\\*\\*")
private val STRIKE = Regex("~~(.+?)~~")
private val ITALIC = Regex("(?<!\\*)\\*([^*]+)\\*(?!\\*)|_([^_]+)_")

private fun fenceOf(line: String): String? {
    val t = line.trim()
    return when {
        t.startsWith("```") -> "```"
        t.startsWith("~~~") -> "~~~"
        else -> null
    }
}

private fun isRule(line: String): Boolean {
    val t = line.trim().replace(" ", "")
    return t.length >= 3 && (t.all { it == '-' } || t.all { it == '*' } || t.all { it == '_' })
}

private fun isTableRow(line: String): Boolean = line.trim().startsWith("|") && line.trim().length > 1

private fun isTableDivider(line: String): Boolean {
    if (!isTableRow(line)) return false
    val cells = splitTableRow(line)
    return cells.isNotEmpty() && cells.all { cell ->
        val t = cell.trim()
        t.isNotEmpty() && t.all { it == '-' || it == ':' } && t.contains('-')
    }
}

private fun splitTableRow(line: String): List<String> =
    line.trim().trim('|').split("|").map { it.trim() }

private fun listMarker(line: String): ListKind? {
    val t = line.trimStart()
    return when {
        // The task box is checked first: `- [ ]` also matches the bullet marker,
        // and a checklist rendered as plain bullets loses the boxes.
        BULLET_MARKER.containsMatchIn(t) && TASK_BOX.containsMatchIn(t.replaceFirst(BULLET_MARKER, "")) ->
            ListKind.TASK

        BULLET_MARKER.containsMatchIn(t) && t.matches(Regex("^[-*+]\\s+.*")) -> ListKind.BULLET

        t.matches(Regex("^\\d{1,9}[.)]\\s+.*")) -> ListKind.ORDERED

        else -> null
    }
}

private fun markerRegex(kind: ListKind): Regex =
    if (kind == ListKind.ORDERED) ORDERED_MARKER else BULLET_MARKER

/** True when a line would go on being read as part of the paragraph above it. */
private fun startsPlainLine(line: String): Boolean =
    listMarker(line) == null && !HEADING.matches(line) && fenceOf(line) == null &&
        !isRule(line) && !line.trimStart().startsWith(">") && !isTableRow(line)
