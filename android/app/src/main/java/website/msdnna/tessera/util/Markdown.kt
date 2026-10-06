package website.msdnna.tessera.util

/** Matches a GFM task-list marker at line start: `- [ ]`, `* [x]`, `1. [X]`, … */
private val TASK_MARKER_RE = Regex("""^([ \t>]*(?:[-*+]|\d{1,9}[.)])[ \t]+\[)([ xX])(])""")

/** Any list marker; a task item is one of these with a `[ ]` box behind it. */
private val LIST_MARKER_RE = Regex("""^([ \t]*)((?:[-*+]|\d{1,9}[.)]))([ \t]+)""")

/** An opening or closing code fence: three or more backticks or tildes. */
private val FENCE_RE = Regex("""^[ \t]*(`{3,}|~{3,})(.*)$""")

/** One nesting level of blockquote prefix. */
private val QUOTE_RE = Regex("""^ {0,3}>[ \t]?""")

/** Columns of indentation that open a code block, and the width of a tab. */
private const val CODE_INDENT = 4

/**
 * Lists the source lines that carry a *rendered* checkbox, in the order the boxes
 * appear in the document.
 *
 * The index RichContent reports on a tap is the position of the tapped `<input>`
 * among the boxes marked.js drew, so this has to agree with marked about which
 * lines are task items — and the gap is not academic:
 *
 *  - `- [ ] …` inside a ``` fence or an indented code block is a code SAMPLE,
 *    drawn as text with no box, and a description that documents a checklist is
 *    exactly where such a sample lives;
 *  - `> - [ ] …` in a blockquote IS drawn as a box;
 *  - `- [ ]x`, with no space after the bracket, and an item with nothing after the
 *    box, are literal text to GFM.
 *
 * Counting every line that looks like a marker drifted from the rendering in both
 * directions, so ticking the first real box rewrote the sample inside the fence
 * and left the tapped box to snap back on the next render.
 *
 * Web answers the same question by asking marked itself (`utils/markdown.js`).
 * There is no Kotlin copy of that parser here — the app's copy is JS inside the
 * WebView and a tap has to be answered synchronously — so the block rules are
 * walked instead, pinned by a table measured off marked in `MiscUtilTest`. The
 * approximation left standing is that an indented code block is recognised
 * relative to the innermost open list item, which is where CommonMark's container
 * rules are subtlest.
 */
internal fun taskMarkerLines(src: String): List<Int> {
    val out = mutableListOf<Int>()
    val scan = BlockScan()
    src.split("\n").forEachIndexed { i, raw -> if (scan.step(stripQuotes(raw))) out += i }
    return out
}

/**
 * Flips the `[ ]`↔`[x]` of the [index]-th *rendered* checkbox in [src] (index =
 * the box order [taskMarkerLines] establishes, which is what the preview reports).
 * Returns [src] unchanged if there is no such box.
 */
fun toggleTaskMarker(src: String, index: Int): String {
    if (src.isEmpty() || index < 0) return src
    val line = taskMarkerLines(src).getOrNull(index) ?: return src
    val lines = src.split("\n").toMutableList()
    val m = TASK_MARKER_RE.find(lines[line]) ?: return src
    val checked = m.groupValues[2].equals("x", ignoreCase = true)
    lines[line] = lines[line].replaceFirst(TASK_MARKER_RE, "$1${if (checked) " " else "x"}$3")
    return lines.joinToString("\n")
}

/** A run of backticks closed by a run of the same length — one inline code span. */
private val CODE_SPAN_RE = Regex("""(`+)[^`]*\1(?!`)""")

/**
 * Returns [src] with everything the renderer draws as code replaced by spaces,
 * character positions intact.
 *
 * Decorations that rewrite the rendered HTML — the mention chips, the `#N` links —
 * are kept away from `<code>`/`<pre>` there (`replaceOutsideCode` in
 * `RichContent.kt`). Anything asking the *source* who was mentioned has to keep
 * away from the same text, or «@Иван» in a shell snippet rings a person who sees
 * no chip at all.
 *
 * Fenced and indented blocks are recognised by the same walk as [taskMarkerLines];
 * inline spans are matched per line, so a span wrapped across a line break stays
 * visible (rare, and the failure is a false chip rather than a blanked paragraph).
 */
internal fun codeMasked(src: String): String {
    val scan = BlockScan()
    return src.split("\n").joinToString("\n") { raw ->
        scan.step(stripQuotes(raw))
        if (scan.lastWasCode) " ".repeat(raw.length) else raw.replace(CODE_SPAN_RE) { " ".repeat(it.value.length) }
    }
}

/**
 * Walks a document line by line, tracking the block context a checkbox needs to
 * survive: an open code fence, an indented code block, and the innermost open list
 * item (whose content column is what an indent is measured against inside a list).
 */
private class BlockScan {
    private var fence = ""
    private var listContent = -1
    private var inCode = false
    private var prevBlank = true
    private var wasBlank = true

    /** Whether the line last fed to [step] was code: fenced content, either of its
     *  delimiters, or an indented block. A blank line is not (it carries nothing). */
    var lastWasCode = false
        private set

    /** Feeds one blockquote-stripped line; true when that line draws a checkbox. */
    fun step(body: String): Boolean {
        lastWasCode = false
        if (consumedByCode(body)) return false
        FENCE_RE.find(body)?.let {
            fence = it.groupValues[1]
            lastWasCode = true
            return false
        }
        val indent = indentWidth(body)
        val marker = LIST_MARKER_RE.find(body)
        if (marker == null) {
            // A block of its own at a column the open item cannot contain ends the list.
            if (wasBlank && indent < maxOf(listContent, 0)) listContent = -1
            return false
        }
        listContent = indent + marker.groupValues[2].length + marker.groupValues[3].length
        return isTaskItem(body)
    }

    /** Blank lines, fenced blocks and indented code never carry a box. */
    private fun consumedByCode(body: String): Boolean {
        if (fence.isNotEmpty()) {
            if (closesFence(body, fence)) fence = ""
            prevBlank = false
            lastWasCode = true
            return true
        }
        if (body.isBlank()) {
            prevBlank = true
            return true
        }
        // Inside a list an indent is measured from the open item's content column:
        // four spaces under `- ` is a nested item, six of them are code.
        val codeAt = maxOf(listContent, 0) + CODE_INDENT
        val indent = indentWidth(body)
        wasBlank = prevBlank
        prevBlank = false
        if (inCode && indent >= codeAt) {
            lastWasCode = true
            return true
        }
        inCode = indent >= codeAt && wasBlank
        lastWasCode = inCode
        return inCode
    }
}

/** True when the line's list marker carries a box GFM actually renders. */
private fun isTaskItem(body: String): Boolean {
    val m = TASK_MARKER_RE.find(body) ?: return false
    val after = body.substring(m.value.length)
    return (after.startsWith(" ") || after.startsWith("\t")) && after.isNotBlank()
}

private fun closesFence(body: String, fence: String): Boolean {
    val m = FENCE_RE.find(body) ?: return false
    val run = m.groupValues[1]
    return run[0] == fence[0] && run.length >= fence.length && m.groupValues[2].isBlank()
}

private fun stripQuotes(line: String): String {
    var s = line
    while (true) {
        val m = QUOTE_RE.find(s) ?: return s
        s = s.substring(m.value.length)
    }
}

private fun indentWidth(s: String): Int {
    var n = 0
    for (ch in s) {
        when (ch) {
            ' ' -> n++
            '\t' -> n += CODE_INDENT - n % CODE_INDENT
            else -> return n
        }
    }
    return n
}
