package website.msdnna.tessera.util

import android.content.res.Resources
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.TaskEvent
import website.msdnna.tessera.data.model.TaskEventData

/*
 * Фраза записи истории задачи — Android-зеркало веба
 * (`frontend/src/utils/taskFeed.js eventText`).
 *
 * Сервер пишет на запись две вещи: `kind` — что случилось — и `data` — факты
 * (на что переименовали, в какую колонку перенесли, поставили срок или убрали).
 * Предложение складывается здесь: раньше Android знал только вид события, поэтому
 * `start`/`estimate`/`relation_removed`/`commands`/`synced` показывались сырым
 * латинским ключом, а снятие срока читалось теми же словами, что и установка.
 *
 * У события, которое несёт цель, две строки — «с целью» и «без цели»: это две
 * отдельные фразы, а не суффикс к одной, потому что слова вокруг цели в разных
 * языках стоят по-разному. Запись, цели не несущая (старая или написанная до
 * появления поля), уходит на «без цели» — фраза с дыркой на месте цели читается
 * как баг. По той же причине у дат и оценки есть нейтральная формулировка:
 * наугад выбранная половина «поставил/убрал» была бы ложью.
 *
 * Засеянная колонка подписывается своим ключом (#2800) — так же, как всюду, где её
 * называют; колонку, которую назвал пользователь, читаем как есть. Провайдер
 * события `synced` подписывается общим словарём источников, чтобы история не
 * завела второе написание имени, которое уже печатают значки связей.
 *
 * [Resources] приходит явным параметром (как в NotificationText.kt и Dates.kt):
 * подписи собираются и вне композиции.
 */

/** События, которым нечего называть: одна фраза на вид. */
private val BARE = mapOf(
    "created" to R.string.task_event_created,
    "description" to R.string.task_event_description,
    "completed" to R.string.task_event_completed,
    "reopened" to R.string.task_event_reopened,
    "recurred" to R.string.task_event_recurred,
    "assigned" to R.string.task_event_assigned,
    "unassigned" to R.string.task_event_unassigned,
    "archived" to R.string.task_event_archived,
    "restored" to R.string.task_event_restored,
    "comment" to R.string.task_event_comment,
)

/** Установили или убрали: `set`, `cleared` и нейтральная фраза на каждый вид. */
private val FLAGGED = mapOf(
    "due" to Triple(R.string.task_event_due_set, R.string.task_event_due_cleared, R.string.task_event_due),
    "start" to Triple(R.string.task_event_start_set, R.string.task_event_start_cleared, R.string.task_event_start),
    "estimate" to Triple(
        R.string.task_event_estimate_set,
        R.string.task_event_estimate_cleared,
        R.string.task_event_estimate,
    ),
)

/** Фраза события, которая дописывается после имени актора. Неизвестный вид (сервер
 *  ушёл вперёд клиента) показываем сырым ключом — честнее, чем промолчать. */
fun journalText(res: Resources, e: TaskEvent?): String {
    if (e == null) return ""
    BARE[e.kind]?.let { return res.getString(it) }
    FLAGGED[e.kind]?.let { return flagged(res, e.data?.set, it) }

    val d = e.data ?: TaskEventData()
    val column = d.to?.let { columnCaption(res, d.toKey, it) }
    val ref = d.related?.toString()
    // Both halves of the attachment pair name the same field, and a `when` branch
    // that wraps would force a blank line between every branch of the block
    // (ktlint blank-line-between-when-conditions) — so the local keeps them short.
    val file = d.filename
    return when (e.kind) {
        "renamed" -> named(res, d.to, R.string.task_event_renamed_to, R.string.task_event_renamed)
        "priority" -> priority(res, d.to)
        "moved" -> named(res, column, R.string.task_event_moved_to, R.string.task_event_moved)
        "relation" -> named(res, ref, R.string.task_event_relation_to, R.string.task_event_relation)
        "relation_removed" -> named(res, ref, R.string.task_event_relation_removed_to, R.string.task_event_relation_removed)
        "tagged" -> named(res, d.tag, R.string.task_event_tagged_with, R.string.task_event_tagged)
        "untagged" -> named(res, d.tag, R.string.task_event_untagged_with, R.string.task_event_untagged)
        "commands" -> commands(res, d)
        "synced" -> res.getString(R.string.task_event_synced, sourceMeta(d.source).label)
        "attachment" -> named(res, file, R.string.task_event_attachment_named, R.string.task_event_attachment)
        "attachment_removed" -> named(res, file, R.string.task_event_attachment_removed_named, R.string.task_event_attachment_removed)
        else -> e.kind
    }
}

/** Фраза с целью, либо фраза без неё — для записи, цели не несущей. */
private fun named(res: Resources, target: String?, withTarget: Int, bare: Int): String {
    val value = target?.trim()?.ifEmpty { null } ?: return res.getString(bare)
    return res.getString(withTarget, value)
}

/** Две половины одного события; запись без флага читается нейтрально. */
private fun flagged(res: Resources, set: Boolean?, strings: Triple<Int, Int, Int>): String {
    val (wasSet, wasCleared, neutral) = strings
    return res.getString(if (set == null) neutral else if (set) wasSet else wasCleared)
}

/** Уровень за пределами шкалы печатаем числом: номер, который пользователь может
 *  назвать, полезнее молчаливого «без приоритета» (паритет с вебом). */
private fun priority(res: Resources, to: String?): String {
    val raw = to?.trim()?.ifEmpty { null } ?: return res.getString(R.string.task_event_priority)
    val label = raw.toIntOrNull()?.let { priorityLabel(res, it) } ?: raw
    return res.getString(R.string.task_event_priority_to, label)
}

/** Одна запись на комментарий, поэтому строка перечисляет команды, а не говорит
 *  «выполнил быстрые команды»: молчаливый `/done` иначе читается как «ничего не
 *  произошло». */
private fun commands(res: Resources, d: TaskEventData): String {
    val applied = slashList(d.applied)
    val failed = slashList(d.failed)
    return when {
        applied.isNotEmpty() && failed.isNotEmpty() -> res.getString(R.string.task_event_commands_mixed, applied, failed)
        failed.isNotEmpty() -> res.getString(R.string.task_event_commands_failed, failed)
        applied.isNotEmpty() -> res.getString(R.string.task_event_commands_applied, applied)
        else -> res.getString(R.string.task_event_commands)
    }
}

private fun slashList(keys: List<String>?): String =
    keys.orEmpty().filter { it.isNotBlank() }.joinToString(", ") { "/$it" }
