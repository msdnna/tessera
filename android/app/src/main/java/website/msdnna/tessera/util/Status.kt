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
 * Neighbours for a status move from the task modal, where the move is a column
 * pick and not a drag — so there is nothing to aim between.
 *
 * A SUBTASK is numbered among its parent's children and changing column does not
 * take it out of that list, so it names the row it already stands on and keeps
 * its place. A TOP-LEVEL card names nothing: the server appends it to the end of
 * the destination column (positionForUnanchoredMove → MaxTaskPositionInColumn).
 *
 * Both clients used to emulate that append by hand, sending the highest-position
 * card of the target column as before_id. The list they picked it from is the
 * board's loaded cards — and the board loads a SPRINT-SCOPED slice of them
 * (BoardViewModel.milestoneScope). So on a scoped board the "tail" was the last
 * card of the sprint, not of the column, and positionBetween(prev, nil) =
 * prev + 65536 — exactly where the next card sits on an untouched ladder. The
 * card landed mid-column sharing a position with a card it knew nothing about:
 * the tie the move endpoint otherwise refuses (rejectSameNeighbour,
 * positionExhausted), neither of which fires when only before_id is given. The
 * server counts the whole column, so leaving the slot to it is both correct under
 * a scope and current with cards this client never loaded (#2932).
 */
fun moveNeighbors(
    taskId: String,
    parentId: String?,
    siblings: List<Task>,
): MoveNeighbors =
    if (parentId != null) siblingNeighbors(siblings, taskId) else MoveNeighbors()
