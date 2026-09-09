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

    /** Профиль сильнее устройства: язык един для всех клиентов, а не для телефона. */
    @Test
    fun `the profile language wins over the device and the system one`() {
        assertThat(resolveLanguage(server = "en", device = "ru", system = "ru")).isEqualTo("en")
        assertThat(resolveLanguage(server = "ru", device = "en", system = "en")).isEqualTo("ru")
    }

    /** До логина профиля нет — работает выбор, сделанный тумблером на экране входа. */
    @Test
    fun `without a profile the pre-login choice wins over the system locale`() {
        assertThat(resolveLanguage(server = null, device = "en", system = "ru")).isEqualTo("en")
        assertThat(resolveLanguage(server = "", device = "ru", system = "en")).isEqualTo("ru")
    }

    /** Первый запуск: не трогали ни профиль, ни тумблер — берём локаль телефона.
     *  Ровно ради этого случая пустое значение НЕ становится «ru» по дороге. */
    @Test
    fun `on a first run the system locale decides`() {
        assertThat(resolveLanguage(server = null, device = null, system = "en-US")).isEqualTo("en")
        assertThat(resolveLanguage(server = "", device = "", system = "ru")).isEqualTo("ru")
    }

    /** Незнакомый язык на любом уровне — это «здесь ответа нет», а не «здесь русский»:
     *  иначе телефон на немецком застрял бы на `de → ru` и не доехал бы до следующего
     *  источника. Русский остаётся только последним словом, когда не помог никто. */
    @Test
    fun `an unsupported value falls through to the next source, not to russian`() {
        assertThat(resolveLanguage(server = "de", device = "en", system = "ru")).isEqualTo("en")
        assertThat(resolveLanguage(server = "de", device = "fr", system = "en-GB")).isEqualTo("en")
        assertThat(resolveLanguage(server = "de", device = "fr", system = "es")).isEqualTo(DEFAULT_LANGUAGE)
        assertThat(resolveLanguage(null, null, null)).isEqualTo(DEFAULT_LANGUAGE)
    }

    /** Тумблер на двух языках — это перебор по кругу, а не «включить английский». */
    @Test
    fun `the toggle cycles through the supported languages`() {
        assertThat(nextLanguage("ru")).isEqualTo("en")
        assertThat(nextLanguage("en")).isEqualTo("ru")
        assertThat(nextLanguage("de")).isEqualTo("en") // незнакомое = ru, значит дальше en
        assertThat(nextLanguage(null)).isEqualTo("en")
    }
}
