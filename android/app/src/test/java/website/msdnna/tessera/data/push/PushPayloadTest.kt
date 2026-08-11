package website.msdnna.tessera.data.push

import com.google.common.truth.Truth.assertThat
import org.junit.Test

class PushPayloadTest {
    private fun payload(vararg pairs: Pair<String, String>) = PushPayload.parse(mapOf(*pairs))

    @Test
    fun `full payload maps every field`() {
        val push = payload(
            "notification_id" to "n-1",
            "kind" to "assigned",
            "title" to "Назначена задача",
            "body" to "Починить чайник",
            "task_id" to "t-1",
            "link" to "https://tessera.msdnna.website",
        )
        assertThat(push).isNotNull()
        assertThat(push!!.id).isEqualTo("n-1")
        assertThat(push.title).isEqualTo("Назначена задача")
        assertThat(push.body).isEqualTo("Починить чайник")
        assertThat(push.taskId).isEqualTo("t-1")
    }

    // A notification not tied to a task (a reminder, an integration sync) still
    // shows — it just opens the app instead of a task.
    @Test
    fun `missing task id is null, not empty`() {
        val push = payload("notification_id" to "n-2", "title" to "Напоминание", "body" to "Пора")
        assertThat(push).isNotNull()
        assertThat(push!!.taskId).isNull()

        val blank = payload("notification_id" to "n-3", "body" to "Пора", "task_id" to "  ")
        assertThat(blank!!.taskId).isNull()
    }

    // Nothing to show → no notification at all, rather than an empty one.
    @Test
    fun `blank body yields null`() {
        assertThat(payload("notification_id" to "n-1", "title" to "T", "body" to "")).isNull()
        assertThat(payload("notification_id" to "n-1", "title" to "T", "body" to "   ")).isNull()
        assertThat(PushPayload.parse(emptyMap())).isNull()
    }

    // The title is cosmetic; a payload without one still delivers the text.
    @Test
    fun `missing title falls back to the app name`() {
        val push = payload("body" to "Текст")
        assertThat(push).isNotNull()
        assertThat(push!!.title).isEqualTo("Tessera")
    }

    // The id is what dedups a push against the copy the open app already got
    // over the socket; without one DeviceNotifier falls back to its old key.
    @Test
    fun `missing notification id is null`() {
        assertThat(payload("body" to "Текст")!!.id).isNull()
        assertThat(payload("notification_id" to "", "body" to "Текст")!!.id).isNull()
    }

    // FCM payloads may carry keys we don't know (added server-side later) —
    // they must not break parsing.
    @Test
    fun `unknown keys are ignored`() {
        val push = payload("body" to "Текст", "who_knows" to "x", "google.sent_time" to "123")
        assertThat(push).isNotNull()
        assertThat(push!!.body).isEqualTo("Текст")
    }
}
