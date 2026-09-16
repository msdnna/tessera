package website.msdnna.tessera.data.realtime

// Предикаты «открытый экран пережил своё содержимое» для доски.
//
// Оба события воркспейс-скоупные: про удаление ЛЮБОГО проекта и ЛЮБОЙ доски
// пространства слышат все открытые доски, поэтому важно только совпадение id.
// И у обоих полезная нагрузка — голое `{id}`: перечитывать нечего, сервер унёс
// с собой колонки, карточки и вложения, так что экран обязан уйти, а не
// перезагружаться в 404.
//
// Вынесено из BoardViewModel отдельным файлом ровно затем, чтобы правило
// проверялось без сокета, префов и живого ViewModel — близнец веба
// (frontend/src/utils/boardEvents.js `boardGone`).

/** Этот проект удалён или перенесён в другое пространство? */
fun projectGone(ev: RealtimeEvent, projectId: String): Boolean =
    ev.type == "project.deleted" && projectId.isNotBlank() && eventId(ev) == projectId

/** Эта доска удалена? */
fun boardGone(ev: RealtimeEvent, boardId: String): Boolean =
    ev.type == "board.deleted" && boardId.isNotBlank() && eventId(ev) == boardId

/**
 * `data.id` события как строка, или null.
 *
 * Не `asString` напрямую: у Gson он бросает на null-е и на объекте/массиве, а
 * это разбор данных из сети внутри колбэка сокета — там исключению взяться
 * неоткуда, кроме как уронить приложение.
 */
private fun eventId(ev: RealtimeEvent): String? =
    ev.data?.get("id")?.takeIf { it.isJsonPrimitive }?.asString
