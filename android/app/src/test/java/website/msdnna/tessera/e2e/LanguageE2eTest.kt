package website.msdnna.tessera.e2e

import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import com.google.common.truth.Truth.assertThat
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.ui.AppRoot
import website.msdnna.tessera.ui.TestTags

/**
 * Язык интерфейса до авторизации (#2855).
 *
 * Отдельным классом, а не внутри [AuthE2eTest]: спек подменяет локаль всему дереву
 * (`AppLocale` пересобирает `Resources` через `createConfigurationContext`), и в общем
 * классе эта подмена доставалась соседним спекам. Свой класс — свой `Application`
 * у Robolectric, поэтому подмена никуда не утекает.
 */
@RunWith(RobolectricTestRunner::class)
// Локаль телефона — часть проверки, а не умолчание раннера: экран входа обязан
// открыться по ней, пока профиля ещё нет.
@Config(qualifiers = "en-rUS")
class LanguageE2eTest {
    private val e2e = E2eRule(login = false)
    private val compose = createComposeRule()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    /**
     * Спек нарочно проверяет не подпись самой кнопки, а **чужой** узел формы: кнопка
     * рисует тег языка вручную и «переключилась» бы даже при мёртвом `AppLocale`.
     * Переехавшая кнопка входа доказывает, что перерисовалось всё дерево.
     */
    @Test
    fun `the language toggle switches the pre-login screen and the choice sticks`() {
        compose.setContent { AppRoot() }
        compose.awaitTag(TestTags.AUTH_LANG_TOGGLE)

        // Свежий DataStore (E2eRule чистит его) + английская локаль телефона:
        // экран входа обязан открыться на ней, а не на русском.
        compose.awaitTextOn(TestTags.AUTH_SUBMIT, "Sign in")
        compose.awaitTextOn(TestTags.AUTH_LANG_TOGGLE, "EN")

        compose.onNodeWithTag(TestTags.AUTH_LANG_TOGGLE).performClick()

        // Кнопка входа — чужой узел, и переехала она только потому, что перерисовалось
        // всё дерево; подпись самого тумблера сменилась бы и при мёртвом `AppLocale`.
        compose.awaitTextOn(TestTags.AUTH_SUBMIT, "Войти")
        compose.awaitTextOn(TestTags.AUTH_LANG_TOGGLE, "RU")

        // Ждать запись обязательно через `awaitServer`: он крутит `compose.waitUntil`,
        // а вместе с ним — диспетчер, на котором живёт корутина обработчика клика. Пара
        // `waitForIdle` + `Thread.sleep` выглядит эквивалентной, но не прокручивает его:
        // с ней тумблер «нажат», а `setLanguage` не начинался вовсе и преф пуст.
        // Выбор лёг в устройство отдельно от кэша профиля, поэтому переживёт и
        // перезапуск, и выход из аккаунта.
        val stored = compose.awaitServer("the pre-login language choice") {
            runBlocking { AppContainer.prefs.languageOverride.first() }.takeIf { it.isNotEmpty() }
        }
        assertThat(stored).isEqualTo("ru")
    }
}
