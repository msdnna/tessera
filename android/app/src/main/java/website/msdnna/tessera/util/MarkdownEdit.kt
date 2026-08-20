package website.msdnna.tessera.util

/**
 * Pure text operations behind the markdown editor's typing behaviour (web
 * `utils/mdEdit.js` parity). Each takes the current text plus its selection and
 * returns the next state, so the fiddly cases (multi-line indent, outdent that
 * must not run past column 0, ordered-list numbering) are unit-testable without
 * composing a text field. `null` means "nothing to do — keep what the platform did".
 */
data class MdEdit(val text: String, val start: Int, val end: Int = start)

/**
 * Typing one of these characters with text selected wraps the selection instead
 * of replacing it (the IME default destroys it). Brackets close with their pair,
 * the rest are symmetric.
 */
val WRAP_PAIRS: Map<Char, Char> = mapOf(
    '(' to ')',
    '[' to ']',
    '{' to '}',
    '<' to '>',
    '"' to '"',
    '\'' to '\'',
    '`' to '`',
    '*' to '*',
    '_' to '_',
    '~' to '~',
)

/**
 * Only brackets, quotes and the backtick auto-close on a single keystroke. The
 * emphasis marks (`*` `_` `~`) are deliberately left out: they still wrap a
 * selection, but auto-pairing them on every keystroke turns "a * b" into "a ** b".
 * `<` is out too — HTML like `<details>` is common and `<>` would fight it.
 */
val AUTOCLOSE_PAIRS: Map<Char, Char> = mapOf(
    '(' to ')',
    '[' to ']',
    '{' to '}',
    '"' to '"',
    '\'' to '\'',
    '`' to '`',
)

private val CLOSERS: Set<Char> = AUTOCLOSE_PAIRS.values.toSet()

/**
 * Two spaces: markdown list nesting in this project counts by two, and the field
 * is monospace so the indent lines up with the rendered nesting.
 */
const val MD_INDENT: String = "  "

/** Wraps [start], [end] with [ch] and its pair, keeping the text selected. */
fun wrapSelection(value: String, start: Int, end: Int, ch: Char): MdEdit? {
    val close = WRAP_PAIRS[ch] ?: return null
    if (start == end) return null
    return MdEdit(
        text = value.substring(0, start) + ch + value.substring(start, end) + close + value.substring(end),
        start = start + 1,
        end = end + 1,
    )
}

/** [from, to) covering every whole line the selection touches. */
private fun lineSpan(value: String, start: Int, end: Int): Pair<Int, Int> {
    val from = value.lastIndexOf('\n', start - 1) + 1
    val to = value.indexOf('\n', end).let { if (it == -1) value.length else it }
    return from to to
}

fun indentLines(value: String, start: Int, end: Int, unit: String = MD_INDENT): MdEdit {
    val (from, to) = lineSpan(value, start, end)
    val lines = value.substring(from, to).split("\n")
    val block = lines.joinToString("\n") { unit + it }
    return MdEdit(
        text = value.substring(0, from) + block + value.substring(to),
        // `start` sits on the first line, `end` on the last — each line before them
        // added `unit`, so the two ends shift by different amounts.
        start = start + unit.length,
        end = end + unit.length * lines.size,
    )
}

fun outdentLines(value: String, start: Int, end: Int, unit: String = MD_INDENT): MdEdit? {
    val (from, to) = lineSpan(value, start, end)
    val strip = Regex("^ {1,${unit.length}}")
    var removedBeforeStart = 0
    var removedTotal = 0
    val block = value.substring(from, to).split("\n").mapIndexed { i, line ->
        val n = strip.find(line)?.value?.length ?: 0
        if (i == 0) removedBeforeStart = n
        removedTotal += n
        line.substring(n)
    }.joinToString("\n")
    if (removedTotal == 0) return null // every line already at column 0
    val nextStart = maxOf(from, start - removedBeforeStart)
    return MdEdit(
        text = value.substring(0, from) + block + value.substring(to),
        start = nextStart,
        end = maxOf(nextStart, end - removedTotal),
    )
}

/**
 * Prefixes every line the selection touches, leaving the whole block selected
 * (web `applyLinePrefix`). [prefix] takes the line and its index so an ordered
 * list can number itself — see [orderedListPrefix].
 */
fun linePrefixLines(value: String, start: Int, end: Int, prefix: (String, Int) -> String): MdEdit {
    val (from, to) = lineSpan(value, start, end)
    val block = value.substring(from, to).split("\n")
        .mapIndexed { i, line -> prefix(line, i) + line }
        .joinToString("\n")
    return MdEdit(value.substring(0, from) + block + value.substring(to), from, from + block.length)
}

/** Literal-prefix overload (heading, bullet, checkbox, quote). */
fun linePrefixLines(value: String, start: Int, end: Int, prefix: String): MdEdit =
    linePrefixLines(value, start, end) { _, _ -> prefix }

/** Numbers the lines of a selection 1., 2., 3. — passed to [linePrefixLines]. */
val orderedListPrefix: (String, Int) -> String = { _, index -> "${index + 1}. " }

/**
 * What typing [ch] at a collapsed caret should do:
 *  - [StepOver] — the closer is already there, just move past it (so "(" then ")"
 *    stays "()" instead of "())");
 *  - [Insert] — insert the pair with the caret between the halves;
 *  - `null` — leave the keystroke alone.
 */
sealed interface AutoClose {
    data class StepOver(val caret: Int) : AutoClose

    data class Insert(val edit: MdEdit) : AutoClose
}

fun autoClose(value: String, caret: Int, ch: Char): AutoClose? {
    val before = value.getOrNull(caret - 1)
    val after = value.getOrNull(caret)
    // Type-over: the same closing char is already right after the caret.
    if (ch in CLOSERS && after == ch) return AutoClose.StepOver(caret + 1)
    val close = AUTOCLOSE_PAIRS[ch] ?: return null
    // Only pair when what follows is nothing, whitespace or a closer — otherwise
    // "(" typed before existing text would trap it inside the pair.
    val okAfter = after == null || after.isWhitespace() || after in ")]}"
    if (!okAfter) return null
    // Symmetric marks (quote / backtick) additionally need a boundary before the
    // caret so an apostrophe inside a word (don't) or a closing quote stays literal.
    if (ch == close) {
        val boundaryBefore = before == null || before.isWhitespace() || before in "([{"
        if (!boundaryBefore) return null
    }
    val text = value.substring(0, caret) + ch + close + value.substring(caret)
    return AutoClose.Insert(MdEdit(text, caret + 1))
}

/**
 * Removes both halves of an empty auto-inserted pair when Backspace lands between
 * them ("(|)" → ""), so auto-closing never leaves an orphan.
 */
fun deletePair(value: String, caret: Int): MdEdit? {
    val before = value.getOrNull(caret - 1) ?: return null
    val after = value.getOrNull(caret) ?: return null
    if (AUTOCLOSE_PAIRS[before] != after) return null
    return MdEdit(value.removeRange(caret - 1, caret + 1), caret - 1)
}

private data class CurrentLine(val start: Int, val end: Int, val text: String)

private fun currentLine(value: String, caret: Int): CurrentLine {
    val start = value.lastIndexOf('\n', caret - 1) + 1
    val end = value.indexOf('\n', caret).let { if (it == -1) value.length else it }
    return CurrentLine(start, end, value.substring(start, end))
}

private val BULLET_RE = Regex("""^(\s*)([-*+])[ \t]+(\[[ xX]\][ \t]+)?(.*)$""")
private val ORDERED_RE = Regex("""^(\s*)(\d+)([.)])[ \t]+(.*)$""")
private val FENCE_RE = Regex("""^(\s*)(`{3,})([^`]*)$""")
private val CLOSED_BELOW_RE = Regex("""(^|\n)[ \t]*`{3,}""")

private class ListItem(
    val indent: String,
    val content: String,
    val bullet: Char? = null,
    val checkbox: Boolean = false,
    val num: Int = 0,
    val delim: Char = '.',
    val ordered: Boolean = false,
)

/** Parses the marker of a bullet / ordered / checkbox line. */
private fun listItem(line: String): ListItem? {
    BULLET_RE.find(line)?.let { m ->
        return ListItem(
            indent = m.groupValues[1],
            bullet = m.groupValues[2][0],
            checkbox = m.groupValues[3].isNotEmpty(),
            content = m.groupValues[4],
        )
    }
    ORDERED_RE.find(line)?.let { m ->
        return ListItem(
            indent = m.groupValues[1],
            num = m.groupValues[2].toIntOrNull() ?: 0,
            delim = m.groupValues[3][0],
            content = m.groupValues[4],
            ordered = true,
        )
    }
    return null
}

/**
 * The next state for a plain Enter at a collapsed caret, or `null` to let an
 * ordinary newline through. Two cases:
 *  - an opening ``` fence with nothing closing it below → drop a closing fence
 *    under the caret (GitHub-style), so code blocks self-close;
 *  - a list item → carry the marker to the next line (numbers increment,
 *    checkboxes reset to unchecked); an empty item ends the list instead.
 */
fun handleEnter(value: String, caret: Int): MdEdit? {
    val line = currentLine(value, caret)
    val fence = FENCE_RE.find(line.text)
    if (fence != null && caret == line.end) {
        val closedBelow = CLOSED_BELOW_RE.containsMatchIn(value.substring(line.end))
        if (!closedBelow) {
            val indent = fence.groupValues[1]
            val ticks = "`".repeat(fence.groupValues[2].length)
            val insert = "\n$indent\n$indent$ticks"
            val inner = caret + 1 + indent.length
            return MdEdit(value.substring(0, caret) + insert + value.substring(caret), inner)
        }
    }
    val item = listItem(line.text) ?: return null
    // Empty item → end the list: clear the marker line, leave the caret on it.
    if (item.content.isBlank()) {
        return MdEdit(value.substring(0, line.start) + value.substring(line.end), line.start)
    }
    val marker = when {
        item.ordered -> "${item.indent}${item.num + 1}${item.delim} "
        item.checkbox -> "${item.indent}${item.bullet} [ ] "
        else -> "${item.indent}${item.bullet} "
    }
    val insert = "\n$marker"
    val at = caret + insert.length
    return MdEdit(value.substring(0, caret) + insert + value.substring(caret), at)
}

/**
 * Re-applies the smart-typing rules to a change the IME already made. Compose's
 * text field reports edits after the fact (a soft keyboard delivers no key events
 * worth intercepting), so instead of a keydown handler we diff the previous state
 * against the one the platform produced and, when it is a single ordinary
 * keystroke we have an opinion about, return the state it *should* have been.
 * `null` = accept the platform's result as-is.
 */
fun applyTyping(prev: String, prevStart: Int, prevEnd: Int, next: String, nextCaret: Int): MdEdit? {
    val s = minOf(prevStart, prevEnd)
    val e = maxOf(prevStart, prevEnd)
    return when {
        e > s -> typedOverSelection(prev, s, e, next, nextCaret)
        next.length == prev.length + 1 && nextCaret == s + 1 -> typedAtCaret(prev, s, next)
        s > 0 && next.length == prev.length - 1 && nextCaret == s - 1 -> backspacedAt(prev, s, next)
        else -> null
    }
}

/** A single character replaced the selection → wrap it instead of dropping it. */
private fun typedOverSelection(prev: String, s: Int, e: Int, next: String, nextCaret: Int): MdEdit? {
    if (next.length != prev.length - (e - s) + 1 || nextCaret != s + 1) return null
    val ch = next.getOrNull(s) ?: return null
    if (next != prev.substring(0, s) + ch + prev.substring(e)) return null
    return wrapSelection(prev, s, e, ch)
}

/** A single character typed at a collapsed caret. */
private fun typedAtCaret(prev: String, s: Int, next: String): MdEdit? {
    val ch = next[s]
    if (next != prev.substring(0, s) + ch + prev.substring(s)) return null
    if (ch == '\n') return handleEnter(prev, s)
    return when (val act = autoClose(prev, s, ch)) {
        is AutoClose.StepOver -> MdEdit(prev, act.caret)
        is AutoClose.Insert -> act.edit
        null -> null
    }
}

/** Backspace over a single character just before a collapsed caret. */
private fun backspacedAt(prev: String, s: Int, next: String): MdEdit? {
    if (next != prev.substring(0, s - 1) + prev.substring(s)) return null
    return deletePair(prev, s)
}
