package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.Task

/*
 * Board-column helpers shared by the task modal (status row, subtask rows) and
 * the kanban card (divergence marker) — port of web `frontend/src/utils/status.js`.
 * Pure functions: no Compose, no network.
 *
 * A column's `position` is a float8 midpoint (see backend positionBetween), so
 * ordering is always by `position` and never by list index — the caller's list
 * may arrive unsorted.
 */

/** Neighbours sent with `PATCH /tasks/:id/move` to pin the landing slot. */
data class MoveNeighbors(val beforeId: String? = null, val afterId: String? = null)

/** Columns ordered by their float8 position. */
fun sortedColumns(columns: List<BoardColumn>): List<BoardColumn> = columns.sortedBy { it.position }

fun columnById(columns: List<BoardColumn>, id: String?): BoardColumn? =
    if (id.isNullOrBlank()) null else columns.find { it.id == id }

/** The column right of [id], or null when [id] is the last one / unknown. */
fun nextColumn(columns: List<BoardColumn>, id: String?): BoardColumn? {
    val list = sortedColumns(columns)
    val i = list.indexOfFirst { it.id == id }
    return if (i < 0) null else list.getOrNull(i + 1)
}

/**
 * The column to show on a subtask that sits in a different column than its
 * parent — null when they agree (a chip on every row would be pure noise) or
 * when the column is unknown (deleted / not loaded → render nothing).
 */
fun divergedColumn(columnId: String?, parentColumnId: String?, columns: List<BoardColumn>): BoardColumn? {
    if (columnId.isNullOrBlank() || parentColumnId.isNullOrBlank() || columnId == parentColumnId) return null
    return columnById(columns, columnId)
}

/**
 * Target column for the "close now" check: the board's configured done column.
 * Null when the board has none — the caller falls back to the plain `completed`
 * flag.
 */
fun doneTarget(columns: List<BoardColumn>, doneColumnId: String?): BoardColumn? =
    columnById(columns, doneColumnId)

/**
 * Neighbours that keep a moved task's place in its sibling order. Without them
 * the backend's positionBetween(nil, nil) returns the constant 65536 and the
 * task jumps within its parent's list. [siblings] is the current ordering (as
 * rendered); the moved task itself is skipped, so its own position never becomes
 * its own neighbour.
 */
fun siblingNeighbors(siblings: List<Task>, id: String): MoveNeighbors {
    val i = siblings.indexOfFirst { it.id == id }
    if (i < 0) return MoveNeighbors()
    val rest = siblings.filter { it.id != id }
    return MoveNeighbors(beforeId = rest.getOrNull(i - 1)?.id, afterId = rest.getOrNull(i)?.id)
}

/**
 * The task sitting last (highest position) in [columnId], self excluded. Sent as
 * before_id when a top-level task changes column, so it lands at the column's end
 * — the same place a drag-and-drop onto empty space would put it. Without it
 * positionBetween(nil, nil) is the constant 65536, which on a populated board
 * means "somewhere near the top", silently reordering the column.
 */
fun columnTail(tasks: List<Task>, columnId: String?, selfId: String?): String? {
    if (columnId.isNullOrBlank()) return null
    var tail: Task? = null
    for (t in tasks) {
        if (t.id == selfId || t.columnId != columnId) continue
        if (tail == null || t.position > tail.position) tail = t
    }
    return tail?.id
}

/**
 * Neighbours for a status move: a subtask holds its place in the parent's list,
 * a top-level task appends to the end of the target column. Either way we send
 * something — bare nulls mean positionBetween(nil, nil) = 65536, which drops the
 * card near the top of the column and quietly reshuffles it.
 */
fun moveNeighbors(
    taskId: String,
    parentId: String?,
    targetColumnId: String,
    siblings: List<Task>,
    topLevelTasks: List<Task>,
): MoveNeighbors =
    if (parentId != null) {
        siblingNeighbors(siblings, taskId)
    } else {
        MoveNeighbors(beforeId = columnTail(topLevelTasks, targetColumnId, taskId))
    }
