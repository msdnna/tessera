package website.msdnna.tessera.util

/** A run of text, flagged as [changed] vs the baseline — for inline diff highlight. */
data class DiffSegment(val text: String, val changed: Boolean)

/**
 * Common-prefix / common-suffix diff (the Android mirror of the web
 * `utils/linediff.diffSegments`): returns the unchanged head, the diverged middle
 * and the unchanged tail of [cur] relative to [base]. Equal strings yield a single
 * unchanged segment. Cheap O(n) — good enough for a title/description compare.
 */
fun diffSegments(base: String?, cur: String?): List<DiffSegment> {
    val b = base ?: ""
    val c = cur ?: ""
    if (b == c) return listOf(DiffSegment(c, false))

    var p = 0
    val maxPrefix = minOf(b.length, c.length)
    while (p < maxPrefix && b[p] == c[p]) p++

    var s = 0
    val maxSuffix = minOf(b.length - p, c.length - p)
    while (s < maxSuffix && b[b.length - 1 - s] == c[c.length - 1 - s]) s++

    val prefix = c.substring(0, p)
    val middle = c.substring(p, c.length - s)
    val suffix = c.substring(c.length - s)
    return buildList {
        if (prefix.isNotEmpty()) add(DiffSegment(prefix, false))
        if (middle.isNotEmpty()) add(DiffSegment(middle, true))
        if (suffix.isNotEmpty()) add(DiffSegment(suffix, false))
    }
}
