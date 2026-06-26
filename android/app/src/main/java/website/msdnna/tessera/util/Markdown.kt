package website.msdnna.tessera.util

/** Matches a GFM task-list marker at line start: `- [ ]`, `* [x]`, `1. [X]`, … */
private val TASK_MARKER_RE = Regex("""^(\s*(?:[-*+]|\d+[.)])\s+\[)([ xX])(])""")

/**
 * Flips the `[ ]`↔`[x]` of the [index]-th GFM task-list item in [src] (index =
 * render order, top to bottom). Mirrors web `utils/markdown.js toggleTaskMarker`,
 * for interactive preview checkboxes. Returns [src] unchanged if not found.
 */
fun toggleTaskMarker(src: String, index: Int): String {
    if (src.isEmpty()) return src
    val lines = src.split("\n").toMutableList()
    var n = -1
    for (i in lines.indices) {
        val m = TASK_MARKER_RE.find(lines[i])
        if (m != null && ++n == index) {
            val checked = m.groupValues[2].equals("x", ignoreCase = true)
            lines[i] = lines[i].replaceFirst(TASK_MARKER_RE, "$1${if (checked) " " else "x"}$3")
            return lines.joinToString("\n")
        }
    }
    return lines.joinToString("\n")
}
