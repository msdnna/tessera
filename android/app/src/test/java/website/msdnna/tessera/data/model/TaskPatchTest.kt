package website.msdnna.tessera.data.model

import com.google.common.truth.Truth.assertThat
import com.google.gson.Gson
import com.google.gson.JsonParser
import org.junit.Test

/**
 * Тело `PATCH /tasks/:id` — tri-state (`optJSON` в backend/handlers/task_ops.go):
 * ключа нет — не трогать, ключ со значением null — очистить.
 *
 * Обе половины были сломаны, и обе тихо: описание уезжало пустой строкой при
 * КАЖДОМ изменении (у карточки доски его нет — списочный payload его не везёт),
 * а «очистить» не уезжало вовсе, потому что Gson по умолчанию выбрасывает
 * null-поля вместе с ключом.
 */
class TaskPatchTest {
    private fun keys(body: String): Set<String> =
        JsonParser.parseString(body).asJsonObject.keySet()

    @Test
    fun `a patch carries only the fields it was given`() {
        val body = taskPatch(completed = Patch.Set(true)).toWireJson()

        assertThat(keys(body)).containsExactly("completed")
    }

    @Test
    fun `an untouched description never reaches the wire`() {
        // Отметка «выполнено» с карточки доски: описания на руках нет, и его
        // отсутствие в теле — единственное, что не даёт серверу его стереть.
        val body = taskPatch(
            title = Patch.Set("название"),
            priority = Patch.Set(2),
            completed = Patch.Set(true),
        ).toWireJson()

        assertThat(keys(body)).doesNotContain("description")
    }

    @Test
    fun `clearing a field sends the key with null`() {
        val body = taskPatch(
            dueDate = Patch.Set(null),
            startDate = Patch.Set(null),
            estimate = Patch.Set(null),
            recurrence = Patch.Set(null),
        ).toWireJson()

        assertThat(keys(body)).containsExactly("due_date", "start_date", "estimate", "recurrence")
        val o = JsonParser.parseString(body).asJsonObject
        assertThat(o["due_date"].isJsonNull).isTrue()
        assertThat(o["start_date"].isJsonNull).isTrue()
        assertThat(o["estimate"].isJsonNull).isTrue()
        assertThat(o["recurrence"].isJsonNull).isTrue()
    }

    @Test
    fun `an empty description is a value, not an absence`() {
        val body = taskPatch(description = Patch.Set("")).toWireJson()

        assertThat(keys(body)).containsExactly("description")
        assertThat(JsonParser.parseString(body).asJsonObject["description"].asString).isEmpty()
    }

    @Test
    fun `a recurrence rule travels whole, tz included`() {
        val rule = Recurrence(freq = "weekly", interval = 2, weekdays = listOf(1, 4), tz = "Europe/Moscow")

        val body = taskPatch(recurrence = Patch.Set(rule)).toWireJson()

        val sent = JsonParser.parseString(body).asJsonObject["recurrence"].asJsonObject
        assertThat(sent["freq"].asString).isEqualTo("weekly")
        assertThat(sent["interval"].asInt).isEqualTo(2)
        assertThat(sent["tz"].asString).isEqualTo("Europe/Moscow")
    }

    @Test
    fun `an empty patch is an empty object`() {
        assertThat(taskPatch().toWireJson()).isEqualTo("{}")
    }

    /** Почему тело сериализуется своим Gson, а не конвертером Retrofit. */
    @Test
    fun `the shared Gson would swallow a clear`() {
        val patch = taskPatch(dueDate = Patch.Set(null))

        assertThat(Gson().toJson(patch)).isEqualTo("{}")
        assertThat(patch.toWireJson()).isEqualTo("""{"due_date":null}""")
    }
}
