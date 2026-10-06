package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.Task

/*
 * "Авто" ordering for the Gantt view: order tasks by their blocking-dependency graph
 * instead of the incoming list order. Port of `frontend/src/utils/dependencyOrder.js`.
 *
 * Pure function — no Compose, no clock, no network — so the walk is unit-testable
 * on its own (it used to live private inside the Gantt composable, where it wasn't).
 */

/**
 * A DFS pre-order over the edges `blocker → blocked`. Roots (tasks with no blocker) are
 * visited in the incoming order; each task is emitted before its dependents, so a chain
 * A→Б prints A, Б, then the next root (e.g. В) — even when В originally sat right after
 * A. Children of a node follow the incoming order; anything left over (a cycle) is
 * appended in incoming order, so every task appears exactly once.
 *
 * A task with more than one blocker waits for the LAST of them rather than riding out on
 * the first: the walk reaches it once per blocker, and all but the final visit turn back.
 * Emitting on the first would put it above a blocker it has not met yet (A→В, Б→В gives
 * A, В, Б), and the Gantt draws finish→start arrows between rows — so that ordering is
 * read off the chart as an arrow running back up it, which is exactly what «Авто» exists
 * to avoid. Cycle members have a blocker that is never emitted, so the sweep at the end
 * forces the first one out and lets the rest follow normally.
 *
 * [edges] are normalised (blocker, blocked) pairs; edges touching a task absent from
 * [tasks] are ignored.
 */
fun topoByDeps(tasks: List<Task>, edges: List<Pair<String, String>>): List<Task> {
    if (tasks.isEmpty() || edges.isEmpty()) return tasks
    val order = tasks.withIndex().associate { (i, t) -> t.id to i }
    val byId = tasks.associateBy { it.id }
    val adj = HashMap<String, MutableList<String>>().apply { tasks.forEach { put(it.id, mutableListOf()) } }
    val blockedBy = HashMap<String, MutableList<String>>().apply { tasks.forEach { put(it.id, mutableListOf()) } }
    for ((blocker, blocked) in edges) {
        if (!byId.containsKey(blocker) || !byId.containsKey(blocked)) continue
        adj[blocker]!!.add(blocked)
        blockedBy[blocked]!!.add(blocker)
    }
    adj.values.forEach { children -> children.sortBy { order[it] ?: 0 } }
    val visited = HashSet<String>()
    val out = ArrayList<Task>(tasks.size)

    // [force] bypasses the readiness gate for the leftover sweep only — without it a
    // cycle waits on itself and its members would drop out of the list entirely.
    fun visit(id: String, force: Boolean) {
        if (id in visited) return
        if (!force && blockedBy[id]?.any { it !in visited } == true) return
        visited.add(id)
        byId[id]?.let { out.add(it) }
        adj[id]?.forEach { visit(it, false) }
    }
    for (t in tasks) if (blockedBy[t.id].isNullOrEmpty()) visit(t.id, false)
    for (t in tasks) visit(t.id, true) // remaining cycle members, in incoming order
    return out
}
