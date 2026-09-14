package website.msdnna.tessera.util

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

/**
 * Каталоги экрана настроек уведомлений — виды событий и минутные пресеты.
 *
 * Обе таблицы раньше лежали приватными @Composable-функциями в файле экрана и
 * успели отстать от сервера и от веба:
 *
 *  * `integration_sync` — настоящий вид маршрутизации (`handlers/gitlab.go` зовёт
 *    `deliverNotification(…, "integration_sync", …)`), веб держит его в
 *    `EVENT_KINDS`. На телефоне чипа не было вовсе, а сводка правила, созданного в
 *    вебе, печатала ключ латиницей посреди русских подписей.
 *  * подводка 2880 («за 2 дня») есть в вебе (`LEAD_VALUES`) и хранится бэкендом
 *    (валидация отбрасывает только отрицательные). Неизвестное значение выпадающий
 *    список показывает прочерком, то есть выставленная подводка читалась как
 *    «не задано».
 */
@RunWith(RobolectricTestRunner::class)
class NotificationOptionsTest {
    private val appContext: Context get() = ApplicationProvider.getApplicationContext()

    private fun res(language: String) = appContext.withLanguage(language).resources

    @Test
    fun `sync routing kind is captioned, not printed as a raw key`() {
        val kinds = listOf("assigned", "integration_sync")
        assertThat(notifKindsCaption(res("ru"), kinds))
            .isEqualTo("Назначения, Синхронизация интеграций")
        assertThat(notifKindsCaption(res("en"), kinds))
            .isEqualTo("Assignments, Integration sync")
    }

    @Test
    fun `two days ahead is a lead the screen can caption`() {
        assertThat(notifLeadLabel(res("ru"), 2880)).isEqualTo("За 2 дня")
        assertThat(notifLeadLabel(res("en"), 2880)).isEqualTo("2 days before")
        assertThat(NOTIF_LEAD_MINUTES).contains(2880)
    }

    @Test
    fun `every catalogued value has a caption in both languages`() {
        for (language in listOf("ru", "en")) {
            val r = res(language)
            for (kind in NOTIF_ROUTE_KINDS) {
                assertThat(notifKindLabel(r, kind)).isNotEqualTo(kind)
            }
            for (m in NOTIF_LEAD_MINUTES) assertThat(notifLeadLabel(r, m)).isNotEqualTo("—")
            for (m in NOTIF_REPEAT_MINUTES) assertThat(notifRepeatLabel(r, m)).isNotEqualTo("—")
            for (m in NOTIF_DIGEST_MINUTES) assertThat(notifDigestLabel(r, m)).isNotEqualTo("—")
        }
    }

    @Test
    fun `captions inside one catalogue are distinct`() {
        // Один и тот же ресурс на два значения — обычная опечатка копипастой, и
        // список с двумя одинаковыми строками не даёт выбрать нужную.
        val r = res("ru")
        assertThat(notifKindOptions(r).map { it.second }.toSet()).hasSize(NOTIF_ROUTE_KINDS.size)
        assertThat(notifLeadOptions(r).map { it.second }.toSet()).hasSize(NOTIF_LEAD_MINUTES.size)
        assertThat(notifRepeatOptions(r).map { it.second }.toSet()).hasSize(NOTIF_REPEAT_MINUTES.size)
        assertThat(notifDigestOptions(r).map { it.second }.toSet()).hasSize(NOTIF_DIGEST_MINUTES.size)
    }

    @Test
    fun `an empty kind list means the route matches anything`() {
        assertThat(notifKindsCaption(res("ru"), null)).isEqualTo("Любые события")
        assertThat(notifKindsCaption(res("en"), emptyList())).isEqualTo("Any events")
    }

    @Test
    fun `a kind newer than this build still shows something`() {
        // Сервер может научиться новому виду раньше, чем обновится APK. Подписать
        // его нечем, но правило по нему отбирает — сводка не должна его терять.
        assertThat(notifKindsCaption(res("ru"), listOf("warp_drive"))).isEqualTo("warp_drive")
    }
}
