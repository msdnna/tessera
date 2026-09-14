package website.msdnna.tessera.util

import android.content.res.Resources
import website.msdnna.tessera.R

/*
 * Каталоги вариантов экрана настроек уведомлений — виды событий для правил
 * маршрутизации и минутные пресеты расписания. Android-зеркало веба
 * (`frontend/src/components/NotificationSettings.vue`: `EVENT_KINDS`,
 * `LEAD_VALUES`, `REPEAT_VALUES`, `DIGEST_VALUES`).
 *
 * Значения — контракт с сервером, подписи — интерфейс, поэтому лежат врозь:
 * список значений это `val`, а подпись берётся из [Resources] на каждый вызов
 * (`val` с текстом застыл бы на языке первой отрисовки — та же причина, по которой
 * таблицы на экране были функциями, а не полями).
 *
 * Каталоги вынесены из файла экрана сюда, потому что приватная @Composable-таблица
 * тестами не покрывается, а отставать от сервера она уже успела: `integration_sync`
 * сервер маршрутизирует (`handlers/gitlab.go` → `deliverNotification(…,
 * "integration_sync", …)`), веб его показывал, а экран — нет, и вид печатался в
 * сводке правила сырым ключом. Подводка «за 2 дня» (2880) хранится бэкендом и
 * выставляется в вебе, а здесь неизвестное значение показывалось как «—», то есть
 * настройка читалась как несуществующая.
 */

/** Заглушка для значения, которого нет в каталоге (сервер новее этой сборки). */
private const val UNKNOWN = "—"

/**
 * Виды уведомлений, по которым правило маршрутизации отбирает доставки. Порядок —
 * тот же, что в вебе, чтобы два клиента перечисляли их одинаково.
 */
val NOTIF_ROUTE_KINDS: List<String> = listOf(
    "assigned",
    "comment",
    "mention",
    "updated",
    "moved",
    "archived",
    "due_soon",
    "reminder",
    "integration_sync",
)

private val KIND_LABELS = mapOf(
    "assigned" to R.string.notif_kind_assigned,
    "comment" to R.string.notif_kind_comment,
    "mention" to R.string.notif_kind_mention,
    "updated" to R.string.notif_kind_updated,
    "moved" to R.string.notif_kind_moved,
    "archived" to R.string.notif_kind_archived,
    "due_soon" to R.string.notif_kind_due_soon,
    "reminder" to R.string.notif_kind_reminder,
    "integration_sync" to R.string.notif_kind_integration_sync,
)

/** Подводка к сроку, минуты до дедлайна. */
val NOTIF_LEAD_MINUTES: List<Int> = listOf(0, 15, 30, 60, 180, 1440, 2880)

private val LEAD_LABELS = mapOf(
    0 to R.string.notif_lead_on_time,
    15 to R.string.notif_lead_15m,
    30 to R.string.notif_lead_30m,
    60 to R.string.notif_lead_1h,
    180 to R.string.notif_lead_3h,
    1440 to R.string.notif_lead_1d,
    2880 to R.string.notif_lead_2d,
)

/** Повтор напоминания о сроке; 0 — не повторять. */
val NOTIF_REPEAT_MINUTES: List<Int> = listOf(0, 60, 180, 360, 1440)

private val REPEAT_LABELS = mapOf(
    0 to R.string.notif_repeat_once,
    60 to R.string.notif_repeat_1h,
    180 to R.string.notif_repeat_3h,
    360 to R.string.notif_repeat_6h,
    1440 to R.string.notif_repeat_1d,
)

/** Окно склейки уведомлений в дайджест; 0 — выключено. */
val NOTIF_DIGEST_MINUTES: List<Int> = listOf(0, 5, 15, 30, 60)

private val DIGEST_LABELS = mapOf(
    0 to R.string.notif_digest_off,
    5 to R.string.notif_digest_5m,
    15 to R.string.notif_digest_15m,
    30 to R.string.notif_digest_30m,
    60 to R.string.notif_digest_1h,
)

/** Подпись вида события. Незнакомый вид показывается ключом: это хуже подписи, но
 *  лучше пустоты — правило по нему всё равно отбирает, и скрыть его нельзя. */
fun notifKindLabel(res: Resources, kind: String): String =
    KIND_LABELS[kind]?.let { res.getString(it) } ?: kind

/** Виды события с подписями — для чипов редактора правила. */
fun notifKindOptions(res: Resources): List<Pair<String, String>> =
    NOTIF_ROUTE_KINDS.map { it to notifKindLabel(res, it) }

/** Строка-сводка правила: перечисление видов или «любые события» для пустого списка. */
fun notifKindsCaption(res: Resources, kinds: List<String>?): String =
    if (kinds.isNullOrEmpty()) {
        res.getString(R.string.notif_route_any_kinds)
    } else {
        kinds.joinToString(res.getString(R.string.notif_ev_list_sep)) { notifKindLabel(res, it) }
    }

private fun minuteLabel(res: Resources, labels: Map<Int, Int>, minutes: Int): String =
    labels[minutes]?.let { res.getString(it) } ?: UNKNOWN

/** Подпись подводки. Значение вне каталога — «—»: подписать его нечем. */
fun notifLeadLabel(res: Resources, minutes: Int): String = minuteLabel(res, LEAD_LABELS, minutes)

fun notifRepeatLabel(res: Resources, minutes: Int): String = minuteLabel(res, REPEAT_LABELS, minutes)

fun notifDigestLabel(res: Resources, minutes: Int): String = minuteLabel(res, DIGEST_LABELS, minutes)

/** Значения с подписями — для выпадающих списков расписания. */
fun notifLeadOptions(res: Resources): List<Pair<Int, String>> =
    NOTIF_LEAD_MINUTES.map { it to notifLeadLabel(res, it) }

fun notifRepeatOptions(res: Resources): List<Pair<Int, String>> =
    NOTIF_REPEAT_MINUTES.map { it to notifRepeatLabel(res, it) }

fun notifDigestOptions(res: Resources): List<Pair<Int, String>> =
    NOTIF_DIGEST_MINUTES.map { it to notifDigestLabel(res, it) }
