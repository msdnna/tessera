package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.Preferences

/**
 * Нормализация префов даты/времени (#2857). Ресурсов тут нет — это чистые данные,
 * поэтому и Robolectric не нужен.
 *
 * Главное, что проверяется: значение `date_format` приходит из общего
 * `user_preferences` и могло быть записано другим клиентом (веб пишет имена
 * пресетов, Android до #2857 писал date-fns-паттерны), поэтому обе формы обязаны
 * резолвиться, а незнакомая — вырождаться в дефолт, а не ронять экран.
 */
class DateFormatsTest {
    @Test
    fun `preset names pass through`() {
        DatePresets.ALL.forEach { assertThat(normalizeDatePreset(it)).isEqualTo(it) }
    }

    @Test
    fun `legacy Android patterns map onto the nearest preset`() {
        assertThat(normalizeDatePreset("dd.MM.yyyy")).isEqualTo(DatePresets.SHORT)
        assertThat(normalizeDatePreset("dd/MM/yyyy")).isEqualTo(DatePresets.SHORT)
        assertThat(normalizeDatePreset("MM/dd/yyyy")).isEqualTo(DatePresets.SHORT)
        assertThat(normalizeDatePreset("yyyy-MM-dd")).isEqualTo(DatePresets.ISO)
    }

    @Test
    fun `unknown and missing values degrade to the default`() {
        assertThat(normalizeDatePreset(null)).isEqualTo(DatePresets.SHORT)
        assertThat(normalizeDatePreset("")).isEqualTo(DatePresets.SHORT)
        assertThat(normalizeDatePreset("EEE, d MMM")).isEqualTo(DatePresets.SHORT)
    }

    @Test
    fun `time format is 24h unless explicitly 12h`() {
        assertThat(normalizeTimeFormat(TIME_12H)).isEqualTo(TIME_12H)
        assertThat(normalizeTimeFormat(TIME_24H)).isEqualTo(TIME_24H)
        assertThat(normalizeTimeFormat(null)).isEqualTo(TIME_24H)
        assertThat(normalizeTimeFormat("h:mm a")).isEqualTo(TIME_24H)
    }

    @Test
    fun `prefs from the profile carry both settings`() {
        val fmt = DateFormatPrefs.of(Preferences(timeFormat = "12h", dateFormat = "yyyy-MM-dd"))
        assertThat(fmt.time12h).isTrue()
        assertThat(fmt.datePreset).isEqualTo(DatePresets.ISO)

        // Дефолтный профиль (Android писал паттерн) — 24 часа и short.
        val legacy = DateFormatPrefs.of(Preferences())
        assertThat(legacy.time12h).isFalse()
        assertThat(legacy.datePreset).isEqualTo(DatePresets.SHORT)
    }
}
