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
import website.msdnna.tessera.data.model.BoardColumn
import website.msdnna.tessera.data.model.LatestRelease
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.ui.AppLocale
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.ui.screens.WhatsNewSheet
import website.msdnna.tessera.ui.theme.TesseraTheme
import website.msdnna.tessera.ui.theme.accentByKey
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewModel
import website.msdnna.tessera.ui.viewmodels.UpdateState
import website.msdnna.tessera.update.WhatsNewEntries
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
                    WhatsNewEntry(
                        version = "1.2.3",
                        date = "2026-08-23",
                        titleRes = R.string.whats_new_title,
                        itemsRes = R.array.task_priority_labels,
                    ),
                ),
                onDismiss = {},
            )
        }
        compose.onNodeWithText("What's new").assertIsDisplayed()
        compose.onNodeWithText("Got it").assertIsDisplayed()
    }

    /** Волна 20: тексты релизов — тоже ресурсы. Рендерится настоящий
     *  [WhatsNewEntries], а не фикстура: ошибка здесь — это забытый ключ в
     *  `values-en` у реальной записи, а не в выдуманной. */
    @Test
    fun `release highlights come from resources in both locales`() {
        render("en") { WhatsNewSheet(releases = WhatsNewEntries, onDismiss = {}) }
        compose.onNodeWithText("Tasks, as on the web").assertIsDisplayed()
        compose.onNodeWithText("Workspace documents").assertIsDisplayed()
        compose.onNodeWithText(
            "On the phone documents are read-only for now — editing stays on the web.",
        ).assertIsDisplayed()
    }

    @Test
    fun `release highlights render in russian`() {
        render("ru") { WhatsNewSheet(releases = WhatsNewEntries, onDismiss = {}) }
        compose.onNodeWithText("Задачи — как в вебе").assertIsDisplayed()
        compose.onNodeWithText(
            "На телефоне документы пока только для чтения — редактирование остаётся в вебе.",
        ).assertIsDisplayed()
    }

    private fun release() = LatestRelease(version = "1.2.3", versionCode = 123)

    private fun available() = UpdateState.Available(release())

    /** Волна 16: состояние ошибки экрана — единственное действие «Попробовать ещё
     *  раз»; сам текст ошибки приходит с сервера и не переводится. */
    @Test
    fun `error state renders its retry action in english`() {
        render("en") { ErrorState(message = "500: context deadline exceeded", onRetry = {}) }
        compose.onNodeWithText("Try again").assertIsDisplayed()
        compose.onNodeWithText("500: context deadline exceeded").assertIsDisplayed()
    }

    @Test
    fun `error state stays russian by default`() {
        render("ru") { ErrorState(message = "500", onRetry = {}) }
        compose.onNodeWithText("Попробовать ещё раз").assertIsDisplayed()
    }

    /** Подписи под лоадером раньше лежали в `val`-списке — тот застыл бы на языке
     *  первой загрузки класса. Проверка идёт через саму функцию по умолчанию:
     *  рендерить `LoadingCaptions` бессмысленно, она молчит первые 5 секунд. */
    @Test
    fun `loading captions read the profile language, not the class load language`() {
        val captions = mutableListOf<String>()
        render("en") { captions += defaultLoadingMessages() }
        compose.waitForIdle()
        assertThat(captions).containsExactly(
            "Trying to reach the server…",
            "This is taking a bit longer than expected…",
            "Still trying to reach the server…",
        ).inOrder()
    }

    /** Волна 16: карточка задачи. Пилюля конфликта — единственная её подпись,
     *  которую видно без открытия меню «⋯». */
    @Test
    fun `task card conflict pill renders in english`() {
        val task = Task(id = "t", columnId = "c", title = "Ansible rollout", number = 7)
        val state = BoardUiState(
            loading = false,
            columns = listOf(BoardColumn(id = "c", name = "In progress")),
            tasks = listOf(task),
        )
        render("en") {
            TaskCard(
                task = task,
                state = state,
                vm = BoardViewModel(),
                onOpen = {},
                conflictTaskIds = setOf("t"),
                onOpenConflict = {},
            )
        }
        compose.onNodeWithText("Conflict").assertIsDisplayed()
    }

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
