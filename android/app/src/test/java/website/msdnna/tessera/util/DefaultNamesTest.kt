package website.msdnna.tessera.util

import android.content.Context
import androidx.compose.material3.Text
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.Workspace
import website.msdnna.tessera.ui.AppLocale

/**
 * Доработка после ревью этапа 7: имена, которые рождаются на сервере, — личное
 * пространство и четыре засеваемые колонки. Они приезжали готовой русской строкой,
 * поэтому английский интерфейс всё равно показывал «К работе» и «Личное
 * пространство»; теперь рядом едет `name_key` (#2800), а подпись собирается из
 * ресурсов читателя.
 *
 * Проверяется ровно граница между двумя случаями: у засеянного имени ключ есть и
 * подпись обязана переехать на язык профиля, у пользовательского ключа нет и имя
 * обязано остаться как есть — «Бэклог» не превращается в «To do» от смены языка.
 *
 * Экран пинится размером: дефолтные 320x470 Robolectric'а обрезают композицию и
 * дают ложно-красный assertIsDisplayed.
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "w411dp-h891dp")
class DefaultNamesTest {
    @get:Rule
    val compose = createComposeRule()

    private val appContext: Context get() = ApplicationProvider.getApplicationContext()

    private fun res(language: String) = appContext.withLanguage(language).resources

    private fun column(key: String?, name: String) = BoardColumn(id = "c1", name = name, nameKey = key)

    private fun workspace(key: String?, name: String) = Workspace(id = "w1", name = name, nameKey = key)

    /** Все четыре засеянные колонки: ключ с сервера → подпись на языке профиля. */
    @Test
    fun `seeded columns are captioned from the reader's catalogue`() {
        val ru = res("ru")
        val en = res("en")
        val seeded = listOf(
            Triple("todo", "К работе", "To do"),
            Triple("in_progress", "В процессе", "In progress"),
            Triple("review", "На рассмотрении", "In review"),
            Triple("done", "Готово", "Done"),
        )
        for ((key, russian, english) in seeded) {
            // Имя с сервера намеренно чужое обоим языкам: подпись обязана прийти из
            // ресурсов по ключу, а не из присланной строки.
            val col = column(key, name = "seeded-$key")
            assertThat(columnCaption(ru, col)).isEqualTo(russian)
            assertThat(columnCaption(en, col)).isEqualTo(english)
        }
    }

    /** Личное пространство — тот же механизм на второй сущности. */
    @Test
    fun `the personal workspace is captioned from the reader's catalogue`() {
        assertThat(workspaceCaption(res("ru"), workspace("personal", "Личное пространство")))
            .isEqualTo("Личное пространство")
        assertThat(workspaceCaption(res("en"), workspace("personal", "Личное пространство")))
            .isEqualTo("Personal space")
    }

    /** Имя, которое завёл пользователь, ключа не имеет — и не переводится ничем. */
    @Test
    fun `a user-chosen name is shown verbatim in every language`() {
        assertThat(columnCaption(res("en"), column(null, "Бэклог"))).isEqualTo("Бэклог")
        assertThat(columnCaption(res("ru"), column(null, "Backlog"))).isEqualTo("Backlog")
        assertThat(workspaceCaption(res("en"), workspace(null, "Домашние дела"))).isEqualTo("Домашние дела")
    }

    /** Сервер новее клиента: незнакомый ключ читается присланной фразой, а не голым
     *  ключом — тот же фолбэк, что в Errors.kt. */
    @Test
    fun `an unknown key falls back to the string the server sent`() {
        assertThat(columnCaption(res("en"), column("blocked", "Заблокировано"))).isEqualTo("Заблокировано")
        assertThat(workspaceCaption(res("en"), workspace("team", "Команда"))).isEqualTo("Команда")
    }

    /** Колонки может не быть (удалена, не догрузилась) — рисуется пусто, не падает. */
    @Test
    fun `a missing entity has an empty caption`() {
        assertThat(columnCaption(res("ru"), null)).isEmpty()
        assertThat(workspaceCaption(res("ru"), null)).isEmpty()
    }

    /**
     * Значок статуса выбирается по ключу, а не по подписи: на английском профиле
     * подпись «In review» под русскую таблицу слов не подходит, и ⅔-пирог пропал бы.
     */
    @Test
    fun `the review glyph follows the key, not the caption`() {
        assertThat(isReviewColumn("review", "seeded-review")).isTrue()
        assertThat(isReviewColumn("todo", "На рассмотрении")).isFalse()
    }

    /** У колонки без ключа опознание осталось по словам имени — web-паритет. */
    @Test
    fun `a keyless column is still recognised by its name`() {
        assertThat(isReviewColumn(null, "На рассмотрении")).isTrue()
        assertThat(isReviewColumn(null, "Review")).isTrue()
        assertThat(isReviewColumn(null, "Бэклог")).isFalse()
    }

    /**
     * Живьём в композиции: подпись собирается из `LocalResources`, который AppLocale
     * подменяет при смене языка профиля. Ловушка тут та же, что у `SortField` и
     * `AccentTheme` в волнах 17–18 — если подпись где-то застынет, второй язык не
     * приедет.
     */
    @Test
    fun `captions in composition follow the profile language`() {
        val language = mutableStateOf("ru")
        val col = column("review", "seeded-review")
        compose.setContent {
            AppLocale(language = language.value) { Text(columnCaption(LocalResources.current, col)) }
        }
        compose.onNodeWithText("На рассмотрении").assertIsDisplayed()

        compose.runOnIdle { language.value = "en" }
        compose.onNodeWithText("In review").assertIsDisplayed()
    }
}
