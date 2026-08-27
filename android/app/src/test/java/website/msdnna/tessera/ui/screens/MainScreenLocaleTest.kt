package website.msdnna.tessera.ui.screens

import androidx.compose.runtime.Composable
import androidx.compose.ui.test.junit4.createComposeRule
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.data.model.Board
import website.msdnna.tessera.ui.AppLocale
import website.msdnna.tessera.ui.resolve
import website.msdnna.tessera.ui.theme.TesseraTheme
import website.msdnna.tessera.ui.theme.accentByKey

/**
 * Оболочка приложения — волна 19 извлечения строк (#2803). Обе проверки об одном:
 * подпись не должна застывать на языке, который стоял в момент вычисления.
 *
 * Заголовок шапки едет [website.msdnna.tessera.ui.UiText]'ом, а сегменты вида
 * собираются в композиции, — поэтому оба резолвятся внутри [AppLocale], живьём,
 * а не сравнением с ресурсами.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "w411dp-h891dp")
class MainScreenLocaleTest {
    @get:Rule
    val compose = createComposeRule()

    private fun render(language: String, content: @Composable () -> Unit) {
        compose.setContent {
            AppLocale(language = language) {
                TesseraTheme(accent = accentByKey("purple"), isDark = false) { content() }
            }
        }
    }

    @Test
    fun `screen titles resolve in the profile language`() {
        val titles = mutableListOf<String>()
        render("en") {
            titles += titleFor(MainDest.Home).resolve()
            titles += titleFor(MainDest.Notes).resolve()
            titles += titleFor(MainDest.Notifications).resolve()
        }
        compose.waitForIdle()
        assertThat(titles).containsExactly("My work", "Notes", "Notifications").inOrder()
    }

    /** Имя доски — данные с сервера: локализовать его нечем, и английский профиль
     *  не должен на него влиять (подводный камень 3 плана #2796). */
    @Test
    fun `a board keeps the name the server sent`() {
        val titles = mutableListOf<String>()
        render("en") { titles += titleFor(MainDest.BoardView(Board(id = "b", name = "Спринт 12"))).resolve() }
        compose.waitForIdle()
        assertThat(titles).containsExactly("Спринт 12")
    }

    /** Раньше сегменты лежали в `val`-списке на уровне файла — тот вычисляется один
     *  раз при загрузке класса и застыл бы на языке первого открытия доски. */
    @Test
    fun `view segments read the profile language, not the class load language`() {
        val labels = mutableListOf<String>()
        render("en") { labels += viewSegments().map { it.label } }
        compose.waitForIdle()
        assertThat(labels)
            .containsExactly("Board", "List", "Calendar", "Timeline", "Gantt", "Matrix")
            .inOrder()
    }
}
