package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.Task

/*
 * Board counters — port of `frontend/src/utils/boardCounts.js` (#2850, #2851).
 *
 * A column header used to print just the number of cards; it now also prints the
 * total with every subtask folded in, so the reader sees how much work the column
 * really holds. The composer bar prints the same pair for the whole board.
 *
 * The parent → children map arrives flat (all nesting levels at once, as the board
 * loads it), so the walk recurses. It also has to guard against a parent chain that
 * loops back on itself: `parent_id` is server data, and a cycle would otherwise
 * recurse until the stack ends.
 */

/** Cards in a lane, and the same cards with all their subtasks folded in. */
data class BoardCount(val tasks: Int, val total: Int) {
    /** The header only shows the bracketed total when there is something to add. */
    val hasSubtasks: Boolean get() = total > tasks
}

private val EMPTY_COUNT = BoardCount(0, 0)

/**
 * Count [tasks] (top-level cards) and their full task tree.
 *
 * @param subtasksByParent parent id → children, every nesting level in one map
 */
fun countWithSubtasks(tasks: List<Task>, subtasksByParent: Map<String, List<Task>>): BoardCount {
    if (tasks.isEmpty()) return EMPTY_COUNT
    val seen = HashSet<String>()
    var count = 0
    var total = 0

    fun walk(id: String) {
        for (child in subtasksByParent[id].orEmpty()) {
            if (child.id.isEmpty() || !seen.add(child.id)) continue
            total += 1
            walk(child.id)
        }
    }

    for (t in tasks) {
        if (t.id.isEmpty() || !seen.add(t.id)) continue
        count += 1
        total += 1
        walk(t.id)
    }
    return BoardCount(count, total)
}
