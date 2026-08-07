package website.msdnna.tessera.util

/**
 * Moves the element at [from] to index [to], shifting the rest — the pure core of a
 * drag-reorder (composer sort levels; web `<draggable>` on `sortLevels`).
 *
 * Out-of-range indices and `from == to` return the receiver unchanged, so a drag
 * that ends where it started (or on a stale index after a concurrent edit) is a
 * no-op instead of a crash.
 */
fun <T> List<T>.moveItem(from: Int, to: Int): List<T> {
    if (from == to) return this
    if (from !in indices || to !in indices) return this
    val out = toMutableList()
    out.add(to, out.removeAt(from))
    return out
}
