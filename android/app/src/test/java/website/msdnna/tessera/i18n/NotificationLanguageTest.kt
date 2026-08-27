package website.msdnna.tessera.i18n

import android.app.NotificationManager
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import com.google.common.truth.Truth.assertWithMessage
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.R
import website.msdnna.tessera.data.realtime.DevicePush
import website.msdnna.tessera.reminders.ReminderNotifications
import website.msdnna.tessera.ui.viewmodels.titleResForKind
import website.msdnna.tessera.util.withLanguage

/**
 * Строки вне Compose (волна 9 #2803). `stringResource` тут недоступен, язык
 * приходит из профиля — проверяем оба места, где это легко потерять.
 */
@RunWith(RobolectricTestRunner::class)
class NotificationLanguageTest {
    private val appContext: Context get() = ApplicationProvider.getApplicationContext()

    private fun channelName(id: String): String? =
        appContext.getSystemService(NotificationManager::class.java)
            ?.getNotificationChannel(id)?.name?.toString()

    /**
     * Главная ловушка волны: канал создаётся один раз, и ранний выход «он уже
     * есть» оставил бы его подпись на языке первого запуска навсегда — смена
     * языка в профиле до системных настроек уведомлений не доехала бы.
     */
    @Test
    fun `switching the language relabels an existing reminders channel`() {
        ReminderNotifications.ensureChannel(appContext, "ru")
        assertThat(channelName(ReminderNotifications.CHANNEL_ID))
            .isEqualTo(appContext.withLanguage("ru").getString(R.string.notif_channel_reminders_name))

        ReminderNotifications.ensureChannel(appContext, "en")
        assertWithMessage("подпись канала застыла на языке первого запуска")
            .that(channelName(ReminderNotifications.CHANNEL_ID))
            .isEqualTo(appContext.withLanguage("en").getString(R.string.notif_channel_reminders_name))
    }

    /** Незнакомый язык — русский, как и везде (`normalizeLanguage`). */
    @Test
    fun `an unknown language falls back to russian`() {
        ReminderNotifications.ensureChannel(appContext, "de")
        assertThat(channelName(ReminderNotifications.CHANNEL_ID))
            .isEqualTo(appContext.withLanguage("ru").getString(R.string.notif_channel_reminders_name))
    }

    /**
     * Заголовок пуша живёт в состоянии идентификатором ресурса: событие может
     * прийти на одном языке, а показаться — уже на другом.
     */
    @Test
    fun `every notification kind maps to its own title resource`() {
        val kinds = listOf("assigned", "comment", "mention", "updated", "moved", "archived", "due_soon", "reminder")
        val ids = kinds.map { titleResForKind(it) }
        assertWithMessage("два вида события делят один заголовок").that(ids).containsNoDuplicates()
        ids.forEach { assertThat(appContext.getString(it)).isNotEmpty() }
    }

    /** Неизвестный сервером вид события не должен ронять показ — остаётся бренд. */
    @Test
    fun `an unknown kind falls back to the app name`() {
        assertThat(titleResForKind("nope")).isEqualTo(R.string.app_name)
    }

    /** Заголовок из ресурса перекрывает пустой текстовый: его резолвит нотифаер. */
    @Test
    fun `a push carries its title as a resource id`() {
        val push = DevicePush("", "текст", null, "id-1", R.string.push_title_mention)
        assertThat(push.titleRes).isEqualTo(R.string.push_title_mention)
        assertThat(appContext.withLanguage("en").getString(push.titleRes!!)).isEqualTo("You were mentioned")
    }
}
