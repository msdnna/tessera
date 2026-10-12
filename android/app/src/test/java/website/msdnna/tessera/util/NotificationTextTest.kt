package website.msdnna.tessera.util

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import com.google.gson.Gson
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.data.model.Notification
import website.msdnna.tessera.data.model.NotificationPayload

/**
 * Уведомление собирается из `payload` на языке читателя — как в вебе
 * (`utils/notificationText.js`) и как на бэкенде для доставок, покидающих
 * приложение (`internal/notify/locale.go`).
 *
 * До этого Android брал поле `text` — готовое РУССКОЕ предложение, которое сервер
 * пишет один раз на всех. В колокольчике англоязычный профиль видел русскую
 * строку, а в системном уведомлении — английский заголовок над русским телом
 * (заголовок уже собирался из ресурсов). Тот же ивент, пришедший фоном по FCM,
 * приезжал целиком на языке получателя: язык уведомления зависел от того, открыто
 * ли приложение.
 *
 * `text` остаётся фолбэком и никуда не девается: у строк до миграции 0065 payload
 * пуст, а событие новее этого APK показать всё равно нечем.
 */
@RunWith(RobolectricTestRunner::class)
class NotificationTextTest {
    private val appContext: Context get() = ApplicationProvider.getApplicationContext()

    private fun res(language: String) = appContext.withLanguage(language).resources

    private fun payload(json: String): NotificationPayload =
        Gson().fromJson(json, NotificationPayload::class.java)

    private fun text(language: String, json: String, fallback: String = LEGACY): String =
        notificationText(res(language), payload(json), fallback)

    @Test
    fun `event reads in the reader's language`() {
        val p = """{"event":"task_assigned","actor":"Алиса","task_number":12,"title":"Починить чайник"}"""
        // Имя актора и заголовок задачи — контент, он едет как есть на любом языке;
        // переезжает только обёртка вокруг них.
        assertThat(text("ru", p)).isEqualTo("Алиса назначил вам задачу #12 «Починить чайник»")
        assertThat(text("en", p)).isEqualTo("Алиса assigned task #12 to you “Починить чайник”")
    }

    @Test
    fun `changed fields keep the catalog order and drop unknown names`() {
        val p = """{"event":"task_updated","actor":"Алиса","task_number":7,"fields":["due","title","warp_drive"]}"""
        assertThat(text("ru", p)).isEqualTo("Алиса изменил(а) задачу #7: название и срок")
        assertThat(text("en", p)).isEqualTo("Алиса updated task #7: title and due date")
    }

    /**
     * Список полей — фраза: перед последним элементом союз, а не запятая, и ровно
     * так же его склеивают две другие доставки того же уведомления — веб через
     * `Intl.ListFormat` (ut-format.spec.js) и бэкенд через `notify.joinList`
     * (locale_test.go). В английском Oxford-запятой нет: веб считает en как en-GB.
     */
    @Test
    fun `the last changed field is joined with a conjunction`() {
        val one = """{"event":"task_updated","actor":"Алиса","task_number":7,"fields":["priority"]}"""
        assertThat(text("ru", one)).isEqualTo("Алиса изменил(а) задачу #7: приоритет")

        val three = """{"event":"task_updated","actor":"Алиса","task_number":7,"fields":["title","priority","due"]}"""
        assertThat(text("ru", three)).isEqualTo("Алиса изменил(а) задачу #7: название, приоритет и срок")
        assertThat(text("en", three)).isEqualTo("Алиса updated task #7: title, priority and due date")
    }

    /** Уведомления v2 (#3048) — те же фразы, что в вебе и в каталоге бэкенда. */
    @Test
    fun `v2 notifications read in the reader's language`() {
        val stage = """{"event":"milestone_closed","actor":"Илья","title":"Спринт 41","done":19,"total":22}"""
        assertThat(text("ru", stage)).isEqualTo("Илья закрыл(а) этап «Спринт 41» — выполнено 19 из 22")
        assertThat(text("en", stage)).isEqualTo("Илья closed stage “Спринт 41” — 19 of 22 done")

        val soon = """{"event":"conference_starting","title":"Синк","minutes":5}"""
        assertThat(text("ru", soon)).isEqualTo("Через 5 мин начнётся встреча «Синк»")
        assertThat(text("en", soon)).isEqualTo("Meeting “Синк” starts in 5 min")

        val conflict = """{"event":"integration_conflict","label":"GitLab","task_number":418,"fields":["description","state"]}"""
        assertThat(text("ru", conflict))
            .isEqualTo("GitLab · конфликт синхронизации в #418 (описание и статус): изменено в обоих местах")

        val doc = """{"event":"document_mention","actor":"Анна","name":"План","excerpt":"глянь"}"""
        assertThat(text("ru", doc)).isEqualTo("Анна упомянул(а) вас в документе «План» «глянь»")
        val chat = """{"event":"conference_mention","actor":"Анна","name":"Стендап"}"""
        assertThat(text("en", chat)).isEqualTo("Анна mentioned you in the chat of “Стендап”")
    }

    @Test
    fun `a seeded column is captioned, a user-named one is verbatim`() {
        val seeded = """{"event":"task_moved","actor":"Алиса","task_number":3,"column":"К работе","column_key":"todo"}"""
        assertThat(text("en", seeded)).isEqualTo("Алиса moved task #3 → “To do”")
        val own = """{"event":"task_moved","actor":"Алиса","task_number":3,"column":"Бэклог"}"""
        assertThat(text("en", own)).isEqualTo("Алиса moved task #3 → “Бэклог”")
    }

    @Test
    fun `a reminder shows the user's own message, and only its default is translated`() {
        assertThat(text("en", """{"event":"reminder","message":"Полить кактус"}""")).isEqualTo("Полить кактус")
        assertThat(text("en", """{"event":"reminder"}""")).isEqualTo("Reminder")
        assertThat(text("ru", """{"event":"reminder"}""")).isEqualTo("Напоминание")
    }

    @Test
    fun `a failure with no cause gets this side's stand-in`() {
        val p = """{"event":"integration_sync_failed","label":"GitLab","seconds":5}"""
        assertThat(text("en", p)).isEqualTo("GitLab: sync failed — unknown error (after 5s)")
        val named = """{"event":"integration_sync_failed","label":"GitLab","reason":"401","seconds":5}"""
        assertThat(text("ru", named)).isEqualTo("GitLab: синхронизация не удалась — 401 (за 5 с)")
    }

    @Test
    fun `a sync duration is worded from seconds`() {
        fun took(seconds: Int) =
            text("en", """{"event":"integration_sync_ok","label":"GitLab","created":1,"updated":2,"seconds":$seconds}""")
        assertThat(took(0)).isEqualTo("GitLab: +1 new, ~2 updated, in less than a second")
        assertThat(took(45)).isEqualTo("GitLab: +1 new, ~2 updated, in 45s")
        assertThat(took(200)).isEqualTo("GitLab: +1 new, ~2 updated, in 3m 20s")
        assertThat(took(3700)).isEqualTo("GitLab: +1 new, ~2 updated, in 1h 1m")
    }

    @Test
    fun `a missing task number reads as a question mark, not a blank`() {
        assertThat(text("en", """{"event":"task_comment","actor":"Алиса"}"""))
            .isEqualTo("Алиса commented on #?")
    }

    @Test
    fun `an empty payload and an unknown event fall back to the stored sentence`() {
        assertThat(text("en", "{}")).isEqualTo(LEGACY)
        assertThat(text("en", """{"event":"task_teleported","actor":"Алиса"}""")).isEqualTo(LEGACY)
    }

    @Test
    fun `a notification renders from its own payload`() {
        val n = Notification(
            id = "n1",
            kind = "assigned",
            text = LEGACY,
            payload = payload("""{"event":"task_assigned","actor":"Алиса","task_number":12}"""),
        )
        assertThat(notificationText(res("en"), n)).isEqualTo("Алиса assigned task #12 to you")
        assertThat(notificationText(res("en"), Notification(id = "n2", text = LEGACY))).isEqualTo(LEGACY)
    }

    private companion object {
        const val LEGACY = "Старое предложение с сервера"
    }
}
