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
 */
class BoardEventsTest {
    private fun ev(type: String, id: String?): RealtimeEvent {
        val data = JsonObject()
        if (id != null) data.addProperty("id", id)
        return RealtimeEvent(scope = "ws-1", type = type, data = data)
    }

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
        assertThat(touchesBoard(ev("tag.created", "t-1"))).isTrue()
        assertThat(touchesBoard(ev("tag.updated", "t-1"))).isTrue()
        // Удаление тега сервер шлёт ОДНИМ событием, без событий по задачам: не
        // перечитаешь — дорожка остаётся на доске вместе со стоящими в ней карточками.
        assertThat(touchesBoard(ev("tag.deleted", "t-1"))).isTrue()
        // Из префиксов собираются подписи дорожек и меню группировки.
        assertThat(touchesBoard(ev("tag_prefixes.updated", null))).isTrue()
    }

    @Test
    fun `теги и этапы на одних правах`() {
        assertThat(touchesBoard(ev("milestone.created", "m-1")))
            .isEqualTo(touchesBoard(ev("tag.created", "t-1")))
    }

    @Test
    fun `содержимое доски перезагружает её`() {
        for (type in listOf("task.updated", "column.moved", "board.updated", "milestone.deleted")) {
            assertThat(touchesBoard(ev(type, "x-1"))).isTrue()
        }
    }

    @Test
    fun `посторонние события доску не трогают`() {
        for (type in listOf("note.created", "notification", "project.updated", "group.moved", "conference.started")) {
            assertThat(touchesBoard(ev(type, "x-1"))).isFalse()
        }
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
