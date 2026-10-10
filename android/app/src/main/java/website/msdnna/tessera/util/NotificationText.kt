package website.msdnna.tessera.util

import android.content.res.Resources
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.Notification
import website.msdnna.tessera.data.model.NotificationPayload

/*
 * Рендер уведомления в язык читателя — Android-зеркало веба
 * (`frontend/src/utils/notificationText.js`).
 *
 * Сервер хранит на строке уведомления две вещи: `payload` — факты события
 * (`{event, actor, task_number, …}`) — и `text`, предложение, которое он по ним
 * собрал. Оно одно на всех получателей и написано по-русски, поэтому фраза
 * складывается здесь, в клиенте: одна и та же строка читается по-русски или
 * по-английски в зависимости от языка профиля.
 *
 * `text` остаётся фолбэком и никуда не девается: у строк, написанных до миграции
 * 0065, payload пуст, а клиент, встретивший событие новее собственной сборки,
 * обязан показать хоть что-то. Оба случая приходят на одну ветку — ровно так же,
 * как `notify.Sentence` на бэкенде и `notificationText` в вебе.
 *
 * [Resources] приходит явным параметром (как в Dates.kt и Estimation.kt): фраза
 * собирается и вне композиции — пуш живёт от события до показа, и готовая строка
 * застыла бы на языке, который стоял в момент события.
 */

/** Поля задачи, которые сервер перечисляет в payload события `task_updated`.
 *  Порядок — тот же, что в каталоге бэкенда (`notify.fieldOrder`), чтобы фраза
 *  читалась одинаково на любой доставке. Незнакомое имя (сервер новее клиента)
 *  выбрасывается, а не показывается сырым. */
private val FIELD_NAMES = linkedMapOf(
    "title" to R.string.notif_ev_field_title,
    "description" to R.string.notif_ev_field_description,
    "priority" to R.string.notif_ev_field_priority,
    "due" to R.string.notif_ev_field_due,
    "start" to R.string.notif_ev_field_start,
    "estimate" to R.string.notif_ev_field_estimate,
    "completed" to R.string.notif_ev_field_completed,
    "reopened" to R.string.notif_ev_field_reopened,
    // Конфликт синхронизации (#3048) называет и статус задачи.
    "state" to R.string.notif_ev_field_state,
)

/** Шаблоны событий. Ключи и порядок подстановок — каталог `notify.bundles`. */
private val EVENTS = mapOf(
    "task_assigned" to R.string.notif_ev_task_assigned,
    "task_updated" to R.string.notif_ev_task_updated,
    "task_moved" to R.string.notif_ev_task_moved,
    "task_archived" to R.string.notif_ev_task_archived,
    "task_comment" to R.string.notif_ev_task_comment,
    "task_thread_reply" to R.string.notif_ev_task_thread_reply,
    "task_mention" to R.string.notif_ev_task_mention,
    "task_due_soon" to R.string.notif_ev_task_due_soon,
    "reminder" to R.string.notif_ev_reminder,
    "integration_sync_ok" to R.string.notif_ev_sync_ok,
    "integration_sync_partial" to R.string.notif_ev_sync_partial,
    "integration_sync_failed" to R.string.notif_ev_sync_failed,
    "conference_invited" to R.string.notif_ev_conference_invited,
    // Уведомления v2 (#3048).
    "milestone_closed" to R.string.notif_ev_milestone_closed,
    "conference_starting" to R.string.notif_ev_conference_starting,
    "integration_conflict" to R.string.notif_ev_integration_conflict,
    "document_mention" to R.string.notif_ev_document_mention,
    "conference_mention" to R.string.notif_ev_conference_mention,
)

/** Фраза для колокольчика и для тела системного уведомления. */
fun notificationText(res: Resources, n: Notification?): String =
    if (n == null) "" else notificationText(res, n.payload, n.text)

/**
 * Собирает фразу из фактов события. [fallback] — предложение, которое сервер уже
 * написал на строке: оно возвращается для пустого payload и для события, которого
 * эта сборка не знает.
 */
fun notificationText(res: Resources, payload: NotificationPayload?, fallback: String): String {
    val event = payload?.event.orEmpty()
    val template = EVENTS[event] ?: return fallback

    // Текст напоминания — собственный текст пользователя, контент, а не интерфейс:
    // он показывается как есть, переводится только пустая заглушка.
    if (event == "reminder") {
        val message = payload?.message?.trim().orEmpty()
        return message.ifEmpty { res.getString(template) }
    }

    val inline = payload?.title?.trim()?.ifEmpty { null } ?: payload?.excerpt?.trim().orEmpty()
    val ctx = if (inline.isEmpty()) "" else res.getString(R.string.notif_ev_ctx, inline)
    val actor = payload?.actor.orEmpty()
    // Номер задачи: «?» вместо пустоты — фраза с дыркой на месте номера читается
    // как баг (тот же приём в вебе и в notify.taskRef).
    val ref = payload?.taskNumber?.toString() ?: res.getString(R.string.notif_ev_no_ref)

    return when (event) {
        "task_updated" -> res.getString(template, actor, ref, fieldList(res, payload?.fields))

        "task_moved" -> res.getString(template, actor, ref, columnCaption(res, payload?.columnKey, payload?.column.orEmpty()))

        "task_due_soon" -> res.getString(template, ref, payload?.title.orEmpty())

        // Приглашение в конференцию номера задачи не несёт — подстановок две.
        "conference_invited" -> res.getString(template, actor, ctx)

        // Уведомления v2 (#3048). Этап и встреча называются всегда (как срок
        // задачи): без названия фраза ни о чём; упоминание вне задачи называет
        // место (`name`), а комментарий идёт контекстом, как у task_mention.
        "milestone_closed" -> res.getString(
            template,
            actor,
            payload?.title.orEmpty(),
            (payload?.done ?: 0).toString(),
            (payload?.total ?: 0).toString(),
        )

        "conference_starting" -> res.getString(template, (payload?.minutes ?: 0).toString(), payload?.title.orEmpty())

        "integration_conflict" -> res.getString(template, payload?.label.orEmpty(), ref, fieldList(res, payload?.fields))

        "document_mention", "conference_mention" -> {
            val excerpt = payload?.excerpt?.trim().orEmpty()
            val quote = if (excerpt.isEmpty()) "" else res.getString(R.string.notif_ev_ctx, excerpt)
            res.getString(template, actor, payload?.name.orEmpty(), quote)
        }

        "integration_sync_ok", "integration_sync_partial" -> res.getString(
            template,
            payload?.label.orEmpty(),
            (payload?.created ?: 0).toString(),
            (payload?.updated ?: 0).toString(),
            duration(res, payload?.seconds ?: 0),
        )

        // Причина отказа — собственный текст интеграции, он едет как есть. Отказ,
        // которому нечего предъявить, причины не несёт вовсе (сервер оставляет ключ
        // пустым), поэтому слово «почему — неизвестно» наше: без него предложение
        // обрывается на тире.
        "integration_sync_failed" -> res.getString(
            template,
            payload?.label.orEmpty(),
            payload?.reason?.trim()?.ifEmpty { null } ?: res.getString(R.string.notif_ev_reason_unknown),
            duration(res, payload?.seconds ?: 0),
        )

        else -> res.getString(template, actor, ref, ctx)
    }
}

/**
 * «название, срок и приоритет» — изменённые поля, в порядке каталога и без
 * незнакомых имён.
 *
 * Перед последним элементом стоит союз, а не запятая: список здесь — фраза, и
 * веб склеивает тот же список через `Intl.ListFormat` (см. `formatList` в
 * utils/format.js и его тест ut-format.spec.js). Одно уведомление доходит до
 * читателя тремя рендерами — колокольчик веба, этот и `notify.Sentence` для
 * доставок наружу, — поэтому запятая на двух из трёх заставляла одну и ту же
 * правку читаться по-разному.
 */
private fun fieldList(res: Resources, fields: List<String>?): String {
    if (fields.isNullOrEmpty()) return ""
    val present = fields.toSet()
    val words = FIELD_NAMES.filterKeys { it in present }.values.map { res.getString(it) }
    if (words.size < 2) return words.firstOrNull().orEmpty()
    return words.dropLast(1).joinToString(res.getString(R.string.notif_ev_list_sep)) +
        res.getString(R.string.notif_ev_list_last) + words.last()
}

/** Длительность синхронизации из числа секунд, которое замерил сервер. Раньше он
 *  присылал её уже написанной словами («3 м 20 с») — перевести это не мог никто. */
private fun duration(res: Resources, seconds: Int): String = when {
    seconds <= 0 -> res.getString(R.string.notif_ev_duration_instant)
    seconds >= 3600 -> res.getString(R.string.notif_ev_duration_hm, seconds / 3600, seconds % 3600 / 60)
    seconds >= 60 -> res.getString(R.string.notif_ev_duration_ms, seconds / 60, seconds % 60)
    else -> res.getString(R.string.notif_ev_duration_s, seconds)
}
