package website.msdnna.tessera.reminders

import android.app.AlarmManager
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone
import kotlinx.coroutines.runBlocking
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows
import org.robolectric.shadows.ShadowAlarmManager
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Reminder
import website.msdnna.tessera.data.repository.AuthRepository

/**
 * Жизненный цикл локальных будильников напоминаний (#2932).
 *
 * На андроиде будильник — это вся доставка напоминания: сервер их не рассылает,
 * а текст уезжает в `PendingIntent` и живёт в системе, а не в приложении. Из
 * этого следуют две вещи, которых на вебе нет вовсе (там напоминания только
 * хранятся): набор заведённых будильников обязан пережить процесс, и выход из
 * аккаунта обязан его снять.
 */
@RunWith(RobolectricTestRunner::class)
class ReminderAlarmLifecycleTest {
    private lateinit var context: Context
    private lateinit var alarms: ShadowAlarmManager

    @Before
    fun setUp() {
        context = ApplicationProvider.getApplicationContext()
        AppContainer.init(context)
        alarms = Shadows.shadowOf(context.getSystemService(AlarmManager::class.java))
        // DataStore живёт файлом и переживает тест — начинаем с чистого набора.
        runBlocking { AppContainer.prefs.setScheduledReminderIds(emptySet()) }
    }

    /** Напоминание на [minutesAhead] минут вперёд — будильнику есть на что вставать. */
    private fun reminder(id: String, minutesAhead: Long, done: Boolean = false) = Reminder(
        id = id,
        remindAt = iso(System.currentTimeMillis() + minutesAhead * MINUTE_MILLIS),
        message = "напоминание $id",
        done = done,
    )

    private fun iso(millis: Long): String =
        SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US)
            .apply { timeZone = TimeZone.getTimeZone("UTC") }
            .format(Date(millis))

    @Test
    fun `выход из аккаунта снимает будильники напоминаний`() = runBlocking<Unit> {
        ReminderScheduler.sync(context, listOf(reminder("a", 10), reminder("b", 20)))
        assertThat(alarms.scheduledAlarms).hasSize(2)

        AuthRepository().clearSession()

        // Иначе личное напоминание всплывёт своим текстом уже после выхода —
        // на общем устройстве у того, кто вошёл следующим.
        assertThat(alarms.scheduledAlarms).isEmpty()
        assertThat(AppContainer.prefs.scheduledReminderIds()).isEmpty()
    }

    @Test
    fun `удалённое напоминание не звонит после перезапуска процесса`() = runBlocking<Unit> {
        ReminderScheduler.sync(context, listOf(reminder("a", 10), reminder("b", 20)))

        // Новый процесс: в памяти у планировщика ничего нет, прошлый набор он
        // обязан узнать из DataStore. «b» удалили с другого устройства.
        ReminderScheduler.sync(context, listOf(reminder("a", 10)))

        assertThat(alarms.scheduledAlarms).hasSize(1)
        assertThat(AppContainer.prefs.scheduledReminderIds()).containsExactly("a")
    }

    @Test
    fun `сделанное напоминание будильника не получает`() = runBlocking<Unit> {
        ReminderScheduler.sync(context, listOf(reminder("a", 10, done = true)))

        assertThat(alarms.scheduledAlarms).isEmpty()
        // Id всё равно помним: снимать с устройства нужно и то, что уже прозвонило
        // и висит в шторке.
        assertThat(AppContainer.prefs.scheduledReminderIds()).containsExactly("a")
    }

    @Test
    fun `повторная уборка на пустом наборе ничего не ломает`() = runBlocking<Unit> {
        ReminderScheduler.cancelAllScheduled(context)
        ReminderScheduler.cancelAllScheduled(context)

        assertThat(alarms.scheduledAlarms).isEmpty()
    }

    private companion object {
        const val MINUTE_MILLIS = 60_000L
    }
}
