package website.msdnna.tessera.util

import android.content.res.Resources
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.DeleteCounts

/*
 * What a delete takes with it, in words (#3014) — port of
 * `frontend/src/utils/deleteCounts.js`: «4 проекта, 23 доски и 612 задач».
 *
 * Zero parts are left out, and an empty string means there is nothing to name
 * (the numbers didn't come, or the container is empty): the dialog then keeps
 * its generic message. The sentence around the list puts it after a colon
 * («Будут удалены: …») — in Russian the verb agrees with the list, and a lone
 * «1 доска» would otherwise read «будут удалены».
 */

/** The parts in the order the confirmation names them. */
enum class CountKind { PROJECTS, BOARDS, TASKS }

/**
 * The phrase for [counts]. [part] renders one count («3 доски»), [join] glues
 * the head of the list to its last part («… и 612 задач»). Kept free of
 * Android resources so the ordering and the zero rule are unit-testable.
 */
fun deleteCountsPhrase(
    counts: DeleteCounts?,
    part: (CountKind, Int) -> String,
    join: (list: String, last: String) -> String,
): String {
    if (counts == null) return ""
    val parts = listOf(
        CountKind.PROJECTS to counts.projects,
        CountKind.BOARDS to counts.boards,
        CountKind.TASKS to counts.tasks,
    ).filter { it.second > 0 }.map { (kind, n) -> part(kind, n) }
    return when (parts.size) {
        0 -> ""
        1 -> parts[0]
        else -> join(parts.dropLast(1).joinToString(", "), parts.last())
    }
}

/** [deleteCountsPhrase] with the app's plurals. */
fun deleteCountsPhrase(res: Resources, counts: DeleteCounts?): String = deleteCountsPhrase(
    counts,
    part = { kind, n ->
        val id = when (kind) {
            CountKind.PROJECTS -> R.plurals.delete_counts_projects
            CountKind.BOARDS -> R.plurals.delete_counts_boards
            CountKind.TASKS -> R.plurals.delete_counts_tasks
        }
        res.getQuantityString(id, n, n)
    },
    join = { list, last -> res.getString(R.string.delete_counts_join, list, last) },
)
