package website.msdnna.tessera.data.model

import com.google.common.truth.Truth.assertThat
import com.google.gson.Gson
import org.junit.Test

/**
 * The recurrence rule's `tz` is the zone its calendar arithmetic runs in on the
 * server — the field that decides which day "every Monday" means. A model that
 * does not know about it drops it on every round-trip, and the due picker rebuilds
 * and re-sends the whole rule, so editing the recurrence of a rule authored in the
 * web client would reset it to UTC and move the whole series.
 */
class RecurrenceTzTest {
    private val gson = Gson()

    @Test
    fun `tz survives a load-and-send round-trip`() {
        val fromServer = """
            {"freq":"weekly","interval":1,"weekdays":[1],"tz":"Europe/Moscow"}
        """.trimIndent()

        val rule = gson.fromJson(fromServer, Recurrence::class.java)
        assertThat(rule.tz).isEqualTo("Europe/Moscow")

        val sentBack = gson.toJson(rule)
        assertThat(gson.fromJson(sentBack, Recurrence::class.java).tz).isEqualTo("Europe/Moscow")
    }

    @Test
    fun `a rule without tz stays without one`() {
        val rule = gson.fromJson("""{"freq":"daily","interval":1}""", Recurrence::class.java)
        assertThat(rule.tz).isNull()
        assertThat(gson.toJson(rule)).doesNotContain("\"tz\"")
    }
}
