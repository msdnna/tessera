package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * `user_preferences.language` пишут все клиенты, поэтому сюда прилетает что угодно —
 * от `en-US` до языка, которого в приложении нет. Всё незнакомое должно осесть на
 * русском, а не уронить экран (ср. #2798, где похожее значение `date_format` роняло
 * профиль целиком).
 */
class LanguagesTest {
    @Test
    fun `known languages pass through`() {
        assertThat(normalizeLanguage("ru")).isEqualTo("ru")
        assertThat(normalizeLanguage("en")).isEqualTo("en")
    }

    @Test
    fun `regional and cased tags fall back to the base language`() {
        assertThat(normalizeLanguage("en-US")).isEqualTo("en")
        assertThat(normalizeLanguage("en_GB")).isEqualTo("en")
        assertThat(normalizeLanguage("RU")).isEqualTo("ru")
        assertThat(normalizeLanguage("  en  ")).isEqualTo("en")
    }

    @Test
    fun `unknown, empty and missing values fall back to russian`() {
        assertThat(normalizeLanguage("de")).isEqualTo(DEFAULT_LANGUAGE)
        assertThat(normalizeLanguage("")).isEqualTo(DEFAULT_LANGUAGE)
        assertThat(normalizeLanguage("   ")).isEqualTo(DEFAULT_LANGUAGE)
        assertThat(normalizeLanguage(null)).isEqualTo(DEFAULT_LANGUAGE)
    }

    @Test
    fun `the default language is one of the supported ones`() {
        assertThat(SupportedLanguages).contains(DEFAULT_LANGUAGE)
    }
}
