package website.msdnna.tessera.ui.components

import android.content.Context
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
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.LatestRelease
import website.msdnna.tessera.ui.AppLocale
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.ui.screens.WhatsNewSheet
import website.msdnna.tessera.ui.theme.TesseraTheme
import website.msdnna.tessera.ui.theme.accentByKey
import website.msdnna.tessera.ui.viewmodels.UpdateState
import website.msdnna.tessera.util.WhatsNewEntry
import website.msdnna.tessera.util.withLanguage

/**
 * Общие компоненты (диалоги, попапы, пикеры) — волна 14 извлечения строк. Подписи
 * кнопок здесь приезжают **значениями по умолчанию** параметров composable-функции:
 * `confirmText: String = stringResource(...)`. Такое значение вычисляется в теле
 * композиции, а не при загрузке класса, — но проверить это дешевле, чем рассуждать,
 * поэтому обе локали рендерятся живьём.
 *
 * Экран пинится размером: дефолтные 320x470 Robolectric'а обрезают композицию снизу
 * и дают ложно-красный assertIsDisplayed (см. [website.msdnna.tessera.ui.AppLocaleTest]).
 */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "w411dp-h891dp")
class ComponentLocaleTest {
    @get:Rule
    val compose = createComposeRule()

    private fun render(language: String, content: @androidx.compose.runtime.Composable () -> Unit) {
        compose.setContent {
            AppLocale(language = language) {
                TesseraTheme(accent = accentByKey("purple"), isDark = false) { content() }
            }
        }
    }

    @Test
    fun `confirm dialog takes its default button labels from the profile language`() {
        render("en") {
            TConfirmDialog(title = "Delete board", message = "This cannot be undone", onConfirm = {}, onDismiss = {})
        }
        compose.onNodeWithText("Delete").assertIsDisplayed()
        compose.onNodeWithText("Cancel").assertIsDisplayed()
    }

    @Test
    fun `confirm dialog stays russian by default`() {
        render("ru") {
            TConfirmDialog(title = "Удалить доску", message = "Действие необратимо", onConfirm = {}, onDismiss = {})
        }
        compose.onNodeWithText("Удалить").assertIsDisplayed()
        compose.onNodeWithText("Отмена").assertIsDisplayed()
    }

    /** Подсказка диалога «введите имя» склеивается с именем через плейсхолдер, и в
     *  английской строке вокруг него свои слова и кавычки. Проверяется на ресурсах, а
     *  не рендером: в этом диалоге живёт поле ввода с мигающим курсором — бесконечная
     *  анимация не даёт композиции уйти в idle, и тест ждал бы её вечно. */
    @Test
    fun `type-to-confirm hint interpolates the name in both languages`() {
        val ctx: Context = ApplicationProvider.getApplicationContext()
        assertThat(ctx.withLanguage("en").getString(R.string.dialog_confirm_by_name_hint, "Atlas"))
            .isEqualTo("Type “Atlas” to confirm:")
        assertThat(ctx.withLanguage("ru").getString(R.string.dialog_confirm_by_name_hint, "Atlas"))
            .isEqualTo("Введите «Atlas» для подтверждения:")
    }

    /** Волна 15: диалог обновления. Номер версии приезжает подстановкой `%1$s`, а
     *  «Доступно обновление»/«Обновить» делятся ключом с подвалом боковой панели —
     *  ровно та пара, которую легко разнести по двум разным ключам. */
    @Test
    fun `update dialog renders in english`() {
        render("en") { UpdateDialog(state = available(), onUpdate = {}, onInstall = {}, onDismiss = {}) }
        compose.onNodeWithText("Update available").assertIsDisplayed()
        compose.onNodeWithText("Version 1.2.3").assertIsDisplayed()
        compose.onNodeWithText("Later").assertIsDisplayed()
        compose.onNodeWithText("Update").assertIsDisplayed()
    }

    @Test
    fun `update dialog stays russian by default`() {
        render("ru") { UpdateDialog(state = available(), onUpdate = {}, onInstall = {}, onDismiss = {}) }
        compose.onNodeWithText("Доступно обновление").assertIsDisplayed()
        compose.onNodeWithText("Версия 1.2.3").assertIsDisplayed()
        compose.onNodeWithText("Позже").assertIsDisplayed()
    }

    /** Сбой скачивания рождается во ViewModel, где `Resources` нет: фолбэк едет
     *  [website.msdnna.tessera.ui.UiText]'ом и резолвится уже здесь, на языке профиля. */
    @Test
    fun `download failure falls back to a localized message`() {
        val failed = UpdateState.Failed(UiText.Res(R.string.update_download_failed), release())
        render("en") { UpdateDialog(state = failed, onUpdate = {}, onInstall = {}, onDismiss = {}) }
        compose.onNodeWithText("Download failed").assertIsDisplayed()
        compose.onNodeWithText("Retry").assertIsDisplayed()
    }

    @Test
    fun `whats new sheet renders in english`() {
        render("en") {
            WhatsNewSheet(
                releases = listOf(
                    WhatsNewEntry(version = "1.2.3", date = "2026-08-23", title = "Release", items = listOf("Item")),
                ),
                onDismiss = {},
            )
        }
        compose.onNodeWithText("What's new").assertIsDisplayed()
        compose.onNodeWithText("Got it").assertIsDisplayed()
    }

    private fun release() = LatestRelease(version = "1.2.3", versionCode = 123)

    private fun available() = UpdateState.Available(release())

    @Test
    fun `theme picker renders in english`() {
        render("en") {
            ThemePicker(
                currentAccent = "purple",
                isDark = false,
                onSelectAccent = {},
                onToggleDark = {},
                onDismiss = {},
            )
        }
        compose.onNodeWithText("Appearance").assertIsDisplayed()
        compose.onNodeWithText("Dark theme").assertIsDisplayed()
    }
}
