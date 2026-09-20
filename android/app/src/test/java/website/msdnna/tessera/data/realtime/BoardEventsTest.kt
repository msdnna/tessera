package website.msdnna.tessera.data.realtime

import com.google.common.truth.Truth.assertThat
import com.google.gson.JsonNull
import com.google.gson.JsonObject
import org.junit.Test

/**
 * Удаление открытой доски (#2932).
 *
 * `DeleteBoard` на сервере уносит доску со всеми её колонками, карточками и
 * вложениями и шлёт в пространство голое `board.deleted {id}`. Раньше андроид
 * пропускал его в общую ветку «префикс board → перезагрузить», и `silentReload`
 * шёл в сеть за колонками доски, которой больше нет. Теперь событие узнаётся до
 * перезагрузки и экран уходит на Home — как уже было сделано для
 * `project.deleted`.
 *
 * Оба события воркспейс-скоупные, так что чужие удаления обязаны проходить
 * мимо: иначе одна удалённая доска выкидывала бы с экрана всех, кто смотрит
 * СВОИ доски того же пространства.
 *
 * Ниже — та же мысль про перезагрузку (`touchesBoard`): событие о ЛЮБОЙ доске
 * пространства долетает до всех открытых, а перезагрузка стоит восьми запросов,
 * поэтому событие, которое называет свою доску, сверяется с открытой. Два
 * исключения — карточка, УХОДЯЩАЯ с доски (событие называет доску, куда она
 * ушла), и `task.relation` с одним концом здесь.
 */
class BoardEventsTest {
    private fun ev(type: String, id: String?): RealtimeEvent {
        val data = JsonObject()
        if (id != null) data.addProperty("id", id)
        return RealtimeEvent(scope = "ws-1", type = type, data = data)
    }

    /** Нагрузка `{id, board_id}` — так сервер шлёт колонки и карточки. */
    private fun onBoard(type: String, boardId: String, id: String = "x-1"): RealtimeEvent {
        val data = JsonObject()
        data.addProperty("id", id)
        data.addProperty("board_id", boardId)
        return RealtimeEvent(scope = "ws-1", type = type, data = data)
    }

    /** `task.relation` называет ОБА конца — доски обеих связанных задач. */
    private fun relation(boardId: String, relatedBoardId: String): RealtimeEvent {
        val data = JsonObject()
        data.addProperty("task_id", "t-1")
        data.addProperty("related_task_id", "t-2")
        data.addProperty("board_id", boardId)
        data.addProperty("related_board_id", relatedBoardId)
        return RealtimeEvent(scope = "ws-1", type = "task.relation", data = data)
    }

    /** Открытая доска в тестах про перезагрузку. */
    private val myBoard = "b-1"

    private fun touches(ev: RealtimeEvent, held: Set<String> = emptySet()): Boolean =
        touchesBoard(ev, myBoard, held::contains)

    @Test
    fun `удаление именно этой доски узнаётся`() {
        assertThat(boardGone(ev("board.deleted", "b-1"), "b-1")).isTrue()
    }

    @Test
    fun `чужая доска того же пространства экран не закрывает`() {
        assertThat(boardGone(ev("board.deleted", "b-2"), "b-1")).isFalse()
    }

    @Test
    fun `другое событие о той же доске — не удаление`() {
        assertThat(boardGone(ev("board.updated", "b-1"), "b-1")).isFalse()
        assertThat(boardGone(ev("column.deleted", "b-1"), "b-1")).isFalse()
    }

    @Test
    fun `пустой id доски ничего не совпадает`() {
        // Экран ещё не загрузился: пустое against пустое — не совпадение.
        assertThat(boardGone(ev("board.deleted", ""), "")).isFalse()
    }

    @Test
    fun `удаление проекта этой доски тоже закрывает экран`() {
        assertThat(projectGone(ev("project.deleted", "p-1"), "p-1")).isTrue()
        assertThat(projectGone(ev("project.deleted", "p-2"), "p-1")).isFalse()
        // Две ветки не путаются между собой.
        assertThat(boardGone(ev("project.deleted", "p-1"), "p-1")).isFalse()
        assertThat(projectGone(ev("board.deleted", "b-1"), "b-1")).isFalse()
    }

    // ── что вообще перезагружает доску ──
    //
    // Теги — дорожки ровно в той же мере, что этапы: при группировке по тегам
    // колонка ЕСТЬ тег. Пока `tag.*` отсеивались, один экран вёл себя по-разному
    // по одной кнопке группировки — по этапам дорожки живые, по тегам мёртвые.

    @Test
    fun `события тегов доску перезагружают`() {
        assertThat(touches(ev("tag.created", "t-1"))).isTrue()
        assertThat(touches(ev("tag.updated", "t-1"))).isTrue()
        // Удаление тега сервер шлёт ОДНИМ событием, без событий по задачам: не
        // перечитаешь — дорожка остаётся на доске вместе со стоящими в ней карточками.
        assertThat(touches(ev("tag.deleted", "t-1"))).isTrue()
        // Из префиксов собираются подписи дорожек и меню группировки.
        assertThat(touches(ev("tag_prefixes.updated", null))).isTrue()
    }

    @Test
    fun `теги и этапы на одних правах`() {
        assertThat(touches(ev("milestone.created", "m-1")))
            .isEqualTo(touches(ev("tag.created", "t-1")))
    }

    @Test
    fun `содержимое доски перезагружает её`() {
        for (type in listOf("task.updated", "task.moved", "column.moved", "column.deleted")) {
            assertThat(touches(onBoard(type, myBoard))).isTrue()
        }
        // У `board.*` нагрузка — сама доска: её id лежит в `id`.
        assertThat(touches(ev("board.updated", myBoard))).isTrue()
        // Этапы доску не называют — перезагружаем, не сверяя.
        assertThat(touches(ev("milestone.deleted", "m-1"))).isTrue()
    }

    @Test
    fun `посторонние события доску не трогают`() {
        for (type in listOf("note.created", "notification", "project.updated", "group.moved", "conference.started")) {
            assertThat(touches(ev(type, "x-1"))).isFalse()
        }
    }

    // ── чужая доска того же пространства ──
    //
    // События воркспейс-скоупные: про каждую карточку и каждую колонку ЛЮБОЙ
    // доски пространства слышат ВСЕ открытые доски. Перезагрузка здесь стоит
    // восьми запросов (silentReload), поэтому событие, которое НАЗЫВАЕТ свою
    // доску, обязано сверяться с открытой.

    @Test
    fun `события чужой доски не перезагружают эту`() {
        for (type in listOf("task.updated", "task.created", "column.created", "column.moved", "column.deleted")) {
            assertThat(touches(onBoard(type, "b-2"))).isFalse()
        }
        // Переименование, иконка, смена завершающей колонки — всё это `board.updated`
        // по чужой доске.
        assertThat(touches(ev("board.updated", "b-2"))).isFalse()
        // Заведённая рядом доска этой не касается вовсе.
        assertThat(touches(ev("board.created", "b-2"))).isFalse()
    }

    @Test
    fun `событие без доски в нагрузке перезагружает`() {
        // Лишнее чтение — безопасный способ ошибиться: сверять не с чем.
        assertThat(touches(ev("task.deleted", "t-9"))).isTrue()
        assertThat(touches(ev("column.updated", null))).isTrue()
    }

    @Test
    fun `экран без доски перезагружается на всём`() {
        assertThat(touchesBoard(onBoard("task.updated", "b-2"), "") { false }).isTrue()
    }

    // ── карточка, уходящая с доски ──

    @Test
    fun `карточку с этой доски унесли — перезагружаем`() {
        // Перенос объявляется задачей уже ПОСЛЕ него: событие называет доску, КУДА
        // она ушла, и от чужой карточки неотличимо. Отсеять — оставить её
        // нарисованной у всех, кроме переносившего.
        val leaving = onBoard("task.moved", "b-2", id = "t-7")
        assertThat(touches(leaving, held = setOf("t-7"))).isTrue()
        // А карточку, которой экран не держал, действительно не наше дело.
        assertThat(touches(leaving, held = setOf("t-8"))).isFalse()
    }

    @Test
    fun `колонка чужой доски карточкой не притворяется`() {
        // Исключение — только для `task.*`: id колонки, случайно совпавший с
        // карточкой, перезагрузку не вызывает.
        assertThat(touches(onBoard("column.deleted", "b-2", id = "t-7"), held = setOf("t-7"))).isFalse()
    }

    // ── связи между задачами ──

    @Test
    fun `связь с одним концом на этой доске перезагружает`() {
        // Стрелка рисуется, только когда ОБА конца здесь, поэтому пара с одним
        // концом — это как раз появление или исчезновение стрелки.
        assertThat(touches(relation(myBoard, "b-2"))).isTrue()
        assertThat(touches(relation("b-2", myBoard))).isTrue()
        // Обе задачи на чужих досках — рисовать нечего.
        assertThat(touches(relation("b-2", "b-3"))).isFalse()
    }

    // ── комментарии ──
    //
    // `task.commented` — единственное событие семейства `task.*`, которое на
    // доске не меняет НИЧЕГО: в строке карточки нет ни счётчика комментариев, ни
    // признака «комментарии есть» (бэкенд handlers/task_list_dto.go — там только
    // has_description), и TaskCard ничего такого не рисует. Потребителей у
    // события нет ни одного: экран задачи realtime вообще не слушает.
    //
    // А нагрузка у него — голое `{task_id}`: ни `board_id`, ни `id`, так что
    // общая сверка «называет свою доску» его не ловит, и до правки каждый коммент
    // в ПРОСТРАНСТВЕ (включая каждый пинг бота) стоил телефону восьми запросов
    // silentReload на всех открытых досках (#2932).

    private fun commented(taskId: String): RealtimeEvent {
        val data = JsonObject()
        data.addProperty("task_id", taskId)
        return RealtimeEvent(scope = "ws-1", type = "task.commented", data = data)
    }

    @Test
    fun `коммент доску не перезагружает`() {
        assertThat(touches(commented("t-1"))).isFalse()
        // Даже про карточку, которую экран держит: держит он её без комментариев.
        assertThat(touches(commented("t-7"), held = setOf("t-7"))).isFalse()
    }

    @Test
    fun `коммент не перезагружает и ещё не загруженный экран`() {
        // Общая поблажка «доска неизвестна — перезагружаем» здесь не нужна:
        // перечитывать из-за коммента нечего и потом.
        assertThat(touchesBoard(commented("t-1"), "") { false }).isFalse()
    }

    @Test
    fun `быстрые команды из коммента приезжают своими событиями`() {
        // Команды внутри коммента («/close», «/assign») доску меняют — но шлют
        // СВОИ события через общий слой apply*, и глушится ровно один тип.
        assertThat(touches(onBoard("task.moved", myBoard, id = "t-1"))).isTrue()
        assertThat(touches(ev("task.tagged", null))).isTrue()
        assertThat(touches(ev("task.updated", "t-1"))).isTrue()
    }

    @Test
    fun `битая нагрузка не роняет разбор`() {
        // Колбэк сокета разбирает данные из сети: у Gson `asString` бросает и на
        // JsonNull, и на объекте — исключению здесь взяться неоткуда, кроме как
        // уронить приложение.
        val noData = RealtimeEvent(scope = "ws-1", type = "board.deleted", data = null)
        assertThat(boardGone(noData, "b-1")).isFalse()

        val nullId = JsonObject().apply { add("id", JsonNull.INSTANCE) }
        assertThat(boardGone(RealtimeEvent(type = "board.deleted", data = nullId), "b-1")).isFalse()

        val objectId = JsonObject().apply { add("id", JsonObject()) }
        assertThat(boardGone(RealtimeEvent(type = "board.deleted", data = objectId), "b-1")).isFalse()
    }
}
