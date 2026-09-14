package website.msdnna.tessera.data.model

import com.google.gson.Gson
import com.google.gson.GsonBuilder
import com.google.gson.JsonElement
import com.google.gson.JsonNull
import com.google.gson.JsonObject
import com.google.gson.JsonPrimitive

/**
 * Одно поле частичного изменения задачи (`PATCH /tasks/:id`).
 *
 * Сервер читает тело как tri-state (`optJSON` в backend/handlers/task_ops.go):
 * ключа в теле НЕТ — поле остаётся как есть; ключ есть со значением `null` —
 * поле очищается; иначе — записывается присланное значение. Дата-класс с
 * обычными полями этого различить не умеет, и раньше разъезжались обе половины:
 *
 *  * `description: String = ""` уходил на сервер при КАЖДОМ изменении, а у
 *    карточки описания на руках нет — списочный payload его не везёт
 *    (см. backend/handlers/task_list_dto.go: едет `has_description`, текст — нет).
 *    Поэтому отметка «выполнено», приоритет, срок или переименование прямо с
 *    карточки доски стирали описание задачи. На вебе тот же баг чинил #2665
 *    (frontend/src/utils/taskPatch.js).
 *  * `null` Gson по умолчанию выбрасывает ВМЕСТЕ с ключом, так что «очистить» до
 *    сервера не доезжало вовсе: «Убрать срок» в пикере, снятие оценки и отмена
 *    повтора молча ничего не меняли.
 */
sealed interface Patch<out T> {
    /** Поле не трогаем — ключа в теле не будет, сервер оставит текущее значение. */
    data object Keep : Patch<Nothing>

    /** Записать [value]; `null` означает «очистить поле». */
    data class Set<out T>(val value: T?) : Patch<T>
}

// Своя копия Gson: у общей (RetrofitClient) `serializeNulls` выключен, а здесь
// ключ со значением null — единственный способ сказать «очистить».
private val patchGson: Gson = GsonBuilder().serializeNulls().create()

/**
 * Тело `PATCH /tasks/:id` из перечисленных полей: в объект попадают только те,
 * что пришли как [Patch.Set], — остальные сервер не тронет.
 */
@Suppress("LongParameterList")
fun taskPatch(
    title: Patch<String> = Patch.Keep,
    description: Patch<String> = Patch.Keep,
    priority: Patch<Int> = Patch.Keep,
    dueDate: Patch<String> = Patch.Keep,
    startDate: Patch<String> = Patch.Keep,
    estimate: Patch<Double> = Patch.Keep,
    completed: Patch<Boolean> = Patch.Keep,
    recurrence: Patch<Recurrence> = Patch.Keep,
): JsonObject {
    val body = JsonObject()
    body.put("title", title) { JsonPrimitive(it) }
    body.put("description", description) { JsonPrimitive(it) }
    body.put("priority", priority) { JsonPrimitive(it) }
    body.put("due_date", dueDate) { JsonPrimitive(it) }
    body.put("start_date", startDate) { JsonPrimitive(it) }
    body.put("estimate", estimate) { JsonPrimitive(it) }
    body.put("completed", completed) { JsonPrimitive(it) }
    body.put("recurrence", recurrence) { patchGson.toJsonTree(it) }
    return body
}

/**
 * JSON, который реально уходит на провод. Отдельно от [taskPatch], потому что
 * значение имеет именно сериализация: `null` в теле — это «очистить», и общий
 * Gson его бы выбросил.
 */
fun JsonObject.toWireJson(): String = patchGson.toJson(this)

private fun <T> JsonObject.put(name: String, field: Patch<T>, encode: (T) -> JsonElement) {
    val set = field as? Patch.Set ?: return
    add(name, set.value?.let(encode) ?: JsonNull.INSTANCE)
}
