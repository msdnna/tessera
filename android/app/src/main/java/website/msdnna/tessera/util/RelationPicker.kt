package website.msdnna.tessera.util

import website.msdnna.tessera.data.model.Relation
import website.msdnna.tessera.data.model.WorkspaceTask

/*
 * Кого предлагать в подсказке вкладки «Связи». Порт правила
 * `frontend/src/components/task/TaskRelationsTab.vue` (linkedNumbers + relGroups).
 *
 * Чистая функция — ни Compose, ни сети, — поэтому правило проверяется юнит-тестом;
 * раньше оно жило прямо в композабле, где не проверялось ничем.
 */

/** Сколько строк показывать в подсказке (web режет список тем же способом). */
const val RELATION_PICKER_LIMIT = 30

/**
 * Номера задач, уже связанных с открытой ТЕМ ЖЕ видом связи.
 *
 * Ключ связи — пара + вид: список приходит уже в системе координат открытой задачи
 * (ListTaskRelations разворачивает входящие строки, blocks ⇄ blocked_by), поэтому
 * строка здесь значит ровно то, что создал бы выбор этого же (номер, вид).
 */
fun linkedRelationNumbers(relations: List<Relation>, kind: String): Set<Long> =
    relations.filter { it.kind == kind }.mapNotNull { it.relatedNumber }.toSet()

/**
 * Кандидаты для подсказки: фильтр по номеру/названию, без самой открытой задачи, без
 * безномерных и без уже связанных выбранным видом связи.
 *
 * Уже связанные именно убираются, а не предлагаются и отклоняются: повторная связка
 * той же пары тем же видом на сервере — полный no-op (applyRelation молчит, если
 * RelationExistsBothWays уже ответил «да»), так что строка была бы кнопкой, которая
 * видимо ничего не делает — список после тапа перечитывается тем же самым. Ровно этот
 * вызов делает web.
 *
 * Пустой запрос — пустой список: на телефоне подсказка разворачивается только под
 * набранное, а не показывает всё рабочее пространство.
 */
fun relationPickerMatches(
    candidates: List<WorkspaceTask>,
    currentTaskId: String,
    relations: List<Relation>,
    kind: String,
    query: String,
    limit: Int = RELATION_PICKER_LIMIT,
): List<WorkspaceTask> {
    val q = query.trim().lowercase()
    if (q.isEmpty()) return emptyList()
    val linked = linkedRelationNumbers(relations, kind)
    return candidates.asSequence()
        .filter { it.id != currentTaskId && it.number != null }
        .filter { it.number !in linked }
        .filter { "#${it.number}".contains(q) || it.title.lowercase().contains(q) }
        .take(limit)
        .toList()
}
