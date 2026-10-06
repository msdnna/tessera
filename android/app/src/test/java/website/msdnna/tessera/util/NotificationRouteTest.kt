package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import com.google.gson.Gson
import org.junit.Test
import website.msdnna.tessera.data.model.Notification
import website.msdnna.tessera.data.model.NotificationPayload

/**
 * Куда ведёт тап по уведомлению — как в вебе (`utils/notificationRoute.js`).
 *
 * Адресов ДВА, и порядок проверки не произволен: приглашение в конференцию не
 * указывает ни на какую задачу вовсе (`notifications.task_id` — внешний ключ на
 * tasks, а звонок не задача), поэтому id звонка едет в `payload.conference_id` и
 * проверяется первым — у такой строки нет доски, на которую можно откатиться.
 *
 * До этого Android знал только про задачу: поля `conference_id` не было ни в
 * модели payload, ни в разборе FCM-пуша, и тап по приглашению в звонок не
 * открывал НИЧЕГО — ни из колокольчика, ни из системного баннера.
 */
class NotificationRouteTest {
    private val gson = Gson()

    private fun payload(json: String): NotificationPayload =
        gson.fromJson(json, NotificationPayload::class.java)

    @Test
    fun `приглашение в конференцию ведёт в звонок`() {
        val n = Notification(
            id = "n-1",
            kind = "conference_invite",
            payload = payload("""{"event":"conference_invited","actor":"Аня","title":"Планёрка","conference_id":"c-7"}"""),
        )
        assertThat(NotificationRoute.of(n)).isEqualTo(NotificationRoute.Conference("c-7"))
    }

    @Test
    fun `звонок выигрывает у задачи`() {
        // У такой строки доски нет, но если бы бэкенд когда-нибудь привязал
        // приглашение и к задаче — открывать всё равно звонок, как в вебе.
        val n = Notification(
            id = "n-2",
            taskId = "t-1",
            taskBoardId = "b-1",
            kind = "conference_invite",
            payload = payload("""{"event":"conference_invited","conference_id":"c-9"}"""),
        )
        assertThat(NotificationRoute.of(n)).isEqualTo(NotificationRoute.Conference("c-9"))
    }

    @Test
    fun `обычное уведомление ведёт в задачу на её доске`() {
        val n = Notification(
            id = "n-3",
            taskId = "t-4",
            taskBoardId = "b-2",
            kind = "task_comment",
            payload = payload("""{"event":"task_comment","task_number":12}"""),
        )
        assertThat(NotificationRoute.of(n)).isEqualTo(NotificationRoute.Task("b-2", "t-4"))
    }

    @Test
    fun `без доски задача не открывается`() {
        // Список не смог подтянуть доску — тап только помечает прочитанным.
        val n = Notification(id = "n-4", taskId = "t-5", kind = "task_comment")
        assertThat(NotificationRoute.of(n)).isEqualTo(NotificationRoute.None)
    }

    @Test
    fun `отчёт интеграции не ведёт никуда`() {
        val n = Notification(
            id = "n-5",
            kind = "integration_sync_ok",
            payload = payload("""{"event":"integration_sync_ok","created":2}"""),
        )
        assertThat(NotificationRoute.of(n)).isEqualTo(NotificationRoute.None)
    }

    @Test
    fun `пустой conference_id не считается адресом`() {
        val n = Notification(
            id = "n-6",
            kind = "conference_invite",
            payload = payload("""{"event":"conference_invited","conference_id":"  "}"""),
        )
        assertThat(NotificationRoute.of(n)).isEqualTo(NotificationRoute.None)
    }

    @Test
    fun `строка без payload ведёт в задачу по-старому`() {
        // До миграции 0065 payload пуст — маршрут по-прежнему берётся из колонок.
        val n = Notification(id = "n-7", taskId = "t-8", taskBoardId = "b-3", kind = "task_assigned")
        assertThat(NotificationRoute.of(n)).isEqualTo(NotificationRoute.Task("b-3", "t-8"))
    }
}
