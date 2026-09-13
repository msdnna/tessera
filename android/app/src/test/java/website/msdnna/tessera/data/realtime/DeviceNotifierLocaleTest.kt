package website.msdnna.tessera.data.realtime

import android.app.Notification
import android.app.NotificationManager
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows.shadowOf
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.NotificationPayload

/**
 * Самый заметный симптом бага: уведомление, пришедшее по сокету (приложение
 * открыто), собиралось из двух половин на разных языках — заголовок уже брался из
 * ресурсов профиля (`titleRes`), а тело подставлялось серверным `text`, которое
 * написано по-русски один раз на всех получателей. В шторке англоязычного профиля
 * получалась строка «Task assigned» над русским предложением.
 *
 * Хуже: тот же самый ивент, прилетевший фоном по FCM, приезжал целиком
 * по-английски — сервер пишет тело доставки на языке получателя
 * (`notify.Sentence`). Язык уведомления зависел от того, открыто ли приложение.
 */
@RunWith(RobolectricTestRunner::class)
class DeviceNotifierLocaleTest {
    private val context: Context get() = ApplicationProvider.getApplicationContext()

    private fun postedBody(push: DevicePush, language: String): String {
        val manager = context.getSystemService(NotificationManager::class.java)
        manager.cancelAll()
        DeviceNotifier.show(context, push, language)
        val posted = shadowOf(manager).activeNotifications.single().notification
        return posted.extras.getString(Notification.EXTRA_TEXT).orEmpty()
    }

    @Test
    fun `a socket push reads in the profile language, title and body alike`() {
        val push = DevicePush(
            title = "",
            body = "Алиса назначил вам задачу #12",
            taskId = null,
            id = "n1",
            titleRes = R.string.push_title_assigned,
            payload = NotificationPayload(event = "task_assigned", actor = "Алиса", taskNumber = 12),
        )
        assertThat(postedBody(push, "en")).isEqualTo("Алиса assigned task #12 to you")
        assertThat(postedBody(push, "ru")).isEqualTo("Алиса назначил вам задачу #12")
    }

    @Test
    fun `an FCM push keeps the sentence the server already wrote`() {
        // Фоновый пуш едет без payload: тело сервер уже написал на языке получателя,
        // переписывать его нечем и незачем.
        val push = DevicePush(title = "Task assigned", body = "Alice assigned task #12 to you", taskId = null, id = "n2")
        assertThat(postedBody(push, "en")).isEqualTo("Alice assigned task #12 to you")
    }
}
