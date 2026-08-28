package website.msdnna.tessera.shots

import android.content.res.Resources
import android.graphics.Bitmap
import android.os.SystemClock
import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusManager
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.test.junit4.ComposeContentTestRule
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.core.graphics.Insets
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import website.msdnna.tessera.e2e.E2eBackend
import website.msdnna.tessera.e2e.E2eRule
import website.msdnna.tessera.e2e.awaitNoTag
import website.msdnna.tessera.e2e.awaitTag
import website.msdnna.tessera.e2e.selectGrouping
import website.msdnna.tessera.ui.AppLocale
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.screens.BoardScreen
import website.msdnna.tessera.ui.screens.DocumentsScreen
import website.msdnna.tessera.ui.screens.MilestonesScreen
import website.msdnna.tessera.ui.screens.NotesScreen
import website.msdnna.tessera.ui.screens.RemindersScreen
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraTheme

/**
 * Takes the help centre's **mobile** screenshots (#2795) on a real device.
 *
 * The manual used to illustrate the app with the web's Playwright shots: a
 * desktop board, a modal window, a mouse-shaped interface. This class is the
 * other half of the platform split — the same scenes as
 * `frontend/e2e/shots/help-shots.spec.js`, photographed where the reader
 * actually is.
 *
 * Not part of `make test-android-instrumented`: it seeds demo-shaped content and
 * writes files, which is not what a smoke tier is for. `make android-shots` runs
 * it by name and pulls the PNGs into `docs/help/assets`.
 *
 * Both themes come out of one mount. The theme is a state the wrapper reads, so
 * flipping it and waiting for idle re-paints the same composition — as opposed to
 * a second `setContent`, which the rule does not allow, or a second test, which
 * would re-seed the backend and hand the two shots different data.
 */
class HelpShotsTest {
    /**
     * Which language the run photographs (#2816), from
     * `-Pandroid.testInstrumentationRunnerArguments.shotsLang`. Russian is the
     * base set and keeps the bare file name — [langSuffix] is what the resolver
     * (`HelpAssets.kt`, `helpAssets.js`) tries first for a reader on a translated
     * article, and what it falls back *from* when a twin has not been shot yet.
     *
     * **Declared before [e2e]:** the rule is built with a display name that
     * depends on it, and Kotlin initialises properties in source order.
     */
    private val lang: String = shotsLang()

    private val langSuffix: String = if (lang == DEFAULT_LANG) "" else ".$lang"

    // The seeded user signs every comment and journal row in the shots, so the
    // screenshot run names them rather than taking E2eRule's fixture default.
    private val e2e = E2eRule(accountName = t("Анна Морозова", "Anna Morozova"))
    private val compose = createAndroidComposeRule<ComponentActivity>()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    private val dark = mutableStateOf(false)

    /**
     * Picks the seed string for the language being photographed.
     *
     * The app's own chrome answers in [lang] because [mount] wraps the content in
     * `AppLocale` — but the *content* comes from the backend, which has no idea a
     * shot is being taken. Without this the English manual would illustrate itself
     * with a Russian board.
     */
    private fun t(ru: String, en: String): String = if (lang == DEFAULT_LANG) ru else en

    /** System bar heights of the current mount — the strips [crop] cuts away. */
    private var chrome: Insets = Insets.NONE

    /** Focus manager of the current mount, published by [mount] for [dismissKeyboard]. */
    private var focus: FocusManager? = null

    @Test
    fun board() {
        val fixture = e2e.fixture
        seedBoardContent()

        mount { BoardScreen(board = fixture.board, workspaceId = fixture.workspace.id) }
        compose.awaitTag(TestTags.boardColumn(fixture.firstColumn.id))

        compose.shoot("board-mobile")
    }

    @Test
    fun boardGroupedByTags() {
        val fixture = e2e.fixture
        seedBoardContent()

        mount { BoardScreen(board = fixture.board, workspaceId = fixture.workspace.id) }
        compose.awaitTag(TestTags.boardColumn(fixture.firstColumn.id))
        compose.selectGrouping(TestTags.BOARD_GROUP_TAGS)

        compose.shoot("board-tags-mobile")
    }

    @Test
    fun boardComposer() {
        val fixture = e2e.fixture
        seedBoardContent()

        mount { BoardScreen(board = fixture.board, workspaceId = fixture.workspace.id) }
        compose.awaitTag(TestTags.boardColumn(fixture.firstColumn.id))
        // Collapsed the bar is one clipped row of dimmed chips, and its own taps go
        // to the expand overlay — the article is about the expanded state, so the
        // shot pays the same first tap the reader does. The overlay disappearing is
        // what "expanded" means here, hence awaitNoTag rather than a tag to await.
        compose.onNodeWithTag(TestTags.BOARD_COMPOSER_EXPAND).performClick()
        compose.awaitNoTag(TestTags.BOARD_COMPOSER_EXPAND)

        compose.shoot("board-composer-mobile")
    }

    @Test
    fun boardCustomize() {
        val fixture = e2e.fixture
        seedBoardContent()

        mount { BoardScreen(board = fixture.board, workspaceId = fixture.workspace.id) }
        compose.awaitTag(TestTags.boardColumn(fixture.firstColumn.id))
        // The gear shares its row with the composer bar and hides while the bar is
        // expanded, so the shot takes the reader's route: collapsed bar, then tap.
        compose.onNodeWithTag(TestTags.BOARD_CUSTOMIZE).performClick()
        compose.awaitTag(TestTags.BOARD_CUSTOMIZE_PANEL)

        compose.shoot("board-customize-mobile")
    }

    /**
     * The column «⋯» menu (`board-columns.android`): rename, the eight swatches,
     * the «Завершающая» switch and the delete row — the article's whole middle in
     * one shot.
     *
     * Photographed on the *first* column deliberately. The switch reads «off»
     * there, which is the state the article explains; on the rightmost column it
     * would be on, and the reader would be looking at the one column whose
     * paragraph says «уже завершающая».
     */
    @Test
    fun boardColumnMenu() {
        val fixture = e2e.fixture
        seedBoardContent()

        mount { BoardScreen(board = fixture.board, workspaceId = fixture.workspace.id) }
        compose.awaitTag(TestTags.boardColumn(fixture.firstColumn.id))
        compose.onNodeWithTag(TestTags.columnMenu(fixture.firstColumn.id)).performClick()
        compose.awaitTag(TestTags.COLUMN_MENU_COLORS)

        compose.shoot("board-column-menu-mobile")
    }

    /**
     * The archive scope (`archive.android`): the amber «Архив» chip in the
     * composer bar and the same columns holding archived cards instead of live
     * ones.
     *
     * `archiveOpen` is a `BoardScreen` parameter, like `tagsOpen` — the entry
     * point is the app bar's overflow, which lives above this screen, so the shot
     * sets the scope directly. The cards are archived through the API rather than
     * through the card menu: the article's scene is an archive someone already
     * filled, and driving three «⋯» → «В архив» → confirm rounds would photograph
     * the same thing after a much longer route.
     */
    @Test
    fun archive() {
        val fixture = e2e.fixture
        seedBoardContent()
        // From the first column only: the phone shows one column at a time, so
        // archiving cards spread across the board would leave the shot's column
        // holding one card and the archive looking empty.
        val shelved = E2eBackend.tasks(fixture).filter { it.columnId == fixture.firstColumn.id }.take(3)
        check(shelved.size == 3) { "seed produced ${shelved.size} tasks to archive" }
        shelved.forEach { E2eBackend.archiveTask(fixture, it.id) }

        mount {
            BoardScreen(board = fixture.board, workspaceId = fixture.workspace.id, archiveOpen = true)
        }
        compose.awaitTag(TestTags.boardColumn(fixture.firstColumn.id))
        // The board renders before the archived list arrives, so the column tag
        // alone would photograph the live board a frame before it is replaced.
        compose.awaitText(shelved.first().title)

        compose.shoot("board-archive-mobile")
    }

    @Test
    fun taskScreen() {
        val fixture = e2e.fixture
        val card = seedBoardContent()

        mount { BoardScreen(board = fixture.board, workspaceId = fixture.workspace.id) }
        compose.awaitTag(TestTags.boardColumn(fixture.firstColumn.id))
        compose.onNodeWithTag(TestTags.taskCard(card.id)).performClick()
        compose.awaitTag(TestTags.TASK_TITLE)

        compose.shoot("task-modal-mobile")
    }

    /**
     * The Комментарии tab with a thread in it (`comments.android`).
     *
     * Seeded server-side rather than typed into the composer: the article's scene
     * is a conversation someone is reading, and the composer only ever produces
     * one comment signed by the reader.
     */
    @Test
    fun taskComments() {
        val fixture = e2e.fixture
        val card = seedBoardContent()
        E2eBackend.createComment(
            fixture,
            card.id,
            t(
                "Макет обновился — кнопка входа переехала под поле пароля.",
                "The mockup changed — the sign-in button moved below the password field.",
            ),
        )
        val root = E2eBackend.comments(fixture, card.id).first()
        E2eBackend.createComment(
            fixture,
            card.id,
            t("Учла, переделала отступы.", "Noted, redid the spacing."),
            parentId = root.id,
        )
        E2eBackend.createComment(
            fixture,
            card.id,
            t("Остался тёмный вариант, доделаю завтра.", "Only the dark variant is left, I'll finish it tomorrow."),
        )

        mount { BoardScreen(board = fixture.board, workspaceId = fixture.workspace.id) }
        compose.awaitTag(TestTags.boardColumn(fixture.firstColumn.id))
        compose.onNodeWithTag(TestTags.taskCard(card.id)).performClick()
        compose.awaitTag(TestTags.TASK_TITLE)
        compose.onNodeWithTag(TestTags.taskTab(TestTags.TASK_TAB_COMMENTS)).performClick()
        compose.awaitTag(TestTags.TASK_COMMENT_INPUT)

        compose.shoot("task-comments-mobile")
    }

    /**
     * The Связи tab (`task-links.android`), showing the three relation kinds the
     * article names — the mirror rows are written by the backend, so linking one
     * way is enough to populate both tasks.
     */
    @Test
    fun taskRelations() {
        val fixture = e2e.fixture
        val card = seedBoardContent()
        val blocker = E2eBackend.createTask(fixture, t("Свести токены темы", "Reconcile the theme tokens"))
        val related = E2eBackend.createTask(fixture, t("Разбор макета в Figma", "Figma mockup walkthrough"))
        val blockerNumber = blocker.number
        val relatedNumber = related.number
        checkNotNull(blockerNumber) { "seeded task has no number" }
        checkNotNull(relatedNumber) { "seeded task has no number" }
        E2eBackend.linkTasks(fixture, card.id, blockerNumber, kind = "blocked_by")
        E2eBackend.linkTasks(fixture, card.id, relatedNumber, kind = "relates")

        mount { BoardScreen(board = fixture.board, workspaceId = fixture.workspace.id) }
        compose.awaitTag(TestTags.boardColumn(fixture.firstColumn.id))
        compose.onNodeWithTag(TestTags.taskCard(card.id)).performClick()
        compose.awaitTag(TestTags.TASK_TITLE)
        compose.onNodeWithTag(TestTags.taskTab(TestTags.TASK_TAB_RELATIONS)).performScrollTo().performClick()
        // The rows arrive with the task's detail request, which the tab does not
        // wait for — anchoring on the tab itself would photograph the empty state.
        compose.awaitTag(TestTags.taskRelationRow(blocker.id))

        compose.shoot("task-relations-mobile")
    }

    /**
     * The История tab (`task-history.android`). The journal is a side effect, so
     * the scene is seeded by *doing* things to the task — renaming it and tagging
     * it — and then photographing what the server logged.
     */
    @Test
    fun taskHistory() {
        val fixture = e2e.fixture
        val card = seedBoardContent()
        E2eBackend.renameTask(fixture, card.id, t("Экран входа: свести с макетом", "Sign-in screen: match the mockup"))
        val ready = E2eBackend.createTag(fixture, t("к ревью", "for review"), "#f0a020")
        E2eBackend.addTaskTag(fixture, card.id, ready.id)
        val newest = E2eBackend.events(fixture, card.id).firstOrNull()
        checkNotNull(newest) { "seeded task logged no events" }

        mount { BoardScreen(board = fixture.board, workspaceId = fixture.workspace.id) }
        compose.awaitTag(TestTags.boardColumn(fixture.firstColumn.id))
        compose.onNodeWithTag(TestTags.taskCard(card.id)).performClick()
        compose.awaitTag(TestTags.TASK_TITLE)
        // Six tabs do not fit a phone: the row scrolls horizontally and История is
        // the last of them, so a bare performClick lands on the clipped edge and
        // silently changes nothing — the shot then times out on an empty journal.
        compose.onNodeWithTag(TestTags.taskTab(TestTags.TASK_TAB_HISTORY)).performScrollTo().performClick()
        compose.awaitTag(TestTags.taskEventRow(newest.id))

        compose.shoot("task-history-mobile")
    }

    /**
     * The description editor with its format toolbar (`markdown-editor.android`).
     *
     * The text is typed rather than seeded: a task that arrives with a description
     * opens the tab in **preview** (`startInPreview = description.isNotBlank()`),
     * and preview is the one state this article is not about — plus it renders
     * through a WebView, which the headless emulator leaves out of the screenshot.
     * An empty description opens in write mode, which is the toolbar, the
     * monospaced field and the shevrons the article explains.
     */
    @Test
    fun markdownEditor() {
        val fixture = e2e.fixture
        val card = seedBoardContent()

        mount { BoardScreen(board = fixture.board, workspaceId = fixture.workspace.id) }
        compose.awaitTag(TestTags.boardColumn(fixture.firstColumn.id))
        compose.onNodeWithTag(TestTags.taskCard(card.id)).performClick()
        compose.awaitTag(TestTags.TASK_TITLE)
        compose.onNodeWithTag(TestTags.TASK_DESCRIPTION).performTextInput(
            t(
                "## Экран входа\n\nКнопка входа переезжает под поле пароля.\n\n1. свести отступы\n2. тёмный вариант",
                "## Sign-in screen\n\nThe sign-in button moves below the password field." +
                    "\n\n1. match the spacing\n2. dark variant",
            ),
        )
        dismissKeyboard()

        compose.shoot("markdown-editor-mobile")
    }

    /**
     * The tag manager (`tags.android`). In the app it opens from the board's «⋮»
     * overflow, which lives in `MainScreen` — above the screen this class mounts.
     * `BoardScreen` takes the flag as a parameter, so the shot sets it directly
     * instead of reproducing the host's app bar.
     */
    @Test
    fun tags() {
        val fixture = e2e.fixture
        seedBoardContent()
        // Prefixed tags on top of the plain ones from the seed: the group header
        // and the «Короткие префиксы» switch below the list only exist once some
        // tag carries a prefix, and both are what the article describes.
        val urgent = t("S: срочно", "S: urgent")
        E2eBackend.createTag(fixture, urgent, "#e0533d")
        E2eBackend.createTag(fixture, t("S: потом", "S: later"), "#9aa0aa")

        mount {
            BoardScreen(board = fixture.board, workspaceId = fixture.workspace.id, tagsOpen = true)
        }
        compose.awaitTag(TestTags.TAG_MANAGER)
        // The dialog renders before the board's tag list arrives, so anchoring on
        // it alone would photograph «тегов пока нет». A pill splits its prefix off,
        // so the row shows the value alone.
        compose.awaitText(urgent.substringAfter(": "))

        compose.shoot("tags-mobile")
    }

    @Test
    fun documents() {
        val fixture = e2e.fixture
        val spec = E2eBackend.createDocument(fixture, t("Требования к релизу", "Release requirements"))
        E2eBackend.createDocument(fixture, t("Протокол встречи", "Meeting notes"))
        E2eBackend.createDocument(fixture, t("Инструкция для новичка", "Onboarding guide"), parentId = spec.id)

        mount { DocumentsScreen(workspaceId = fixture.workspace.id) }
        compose.awaitTag(TestTags.documentRow(spec.id))

        compose.shoot("documents-mobile")
    }

    @Test
    fun notes() {
        val fixture = e2e.fixture
        val recap = t("Итоги созвона", "Call recap")
        E2eBackend.createNote(
            fixture,
            recap,
            t(
                "— решили выкатывать в пятницу\n— Аня готовит миграцию",
                "— agreed to ship on Friday\n— Anna is preparing the migration",
            ),
        )
        E2eBackend.createNote(fixture, t("Вопросы к заказчику", "Questions for the client"))
        E2eBackend.createNote(fixture, t("Черновик анонса", "Announcement draft"))

        mount {
            NotesScreen(
                workspaceId = fixture.workspace.id,
                preselectNoteId = null,
                onPreselectConsumed = {},
            )
        }
        compose.awaitText(recap)

        compose.shoot("notes-mobile")
    }

    @Test
    fun reminders() {
        val fixture = e2e.fixture
        val call = t("Позвонить в банк", "Call the bank")
        E2eBackend.createReminder(fixture, call, "2026-09-01T10:00:00Z")
        E2eBackend.createReminder(fixture, t("Отправить отчёт", "Send the report"), "2026-09-02T09:30:00Z")

        mount { RemindersScreen() }
        compose.awaitText(call)

        compose.shoot("reminders-mobile")
    }

    @Test
    fun milestones() {
        val fixture = e2e.fixture
        val releaseName = t("Релиз 1.0", "Release 1.0")
        val release = E2eBackend.createMilestone(
            fixture,
            releaseName,
            startDate = "2026-08-01T00:00:00Z",
            dueDate = "2026-09-15T00:00:00Z",
        )
        E2eBackend.createMilestone(fixture, t("Демо заказчику", "Client demo"), dueDate = "2026-10-01T00:00:00Z")
        // A milestone with no tasks shows «нет задач» instead of a progress bar,
        // so one of the two gets tasks — the shot should show both states.
        val done = E2eBackend.createTask(
            fixture,
            t("Собрать релизные заметки", "Write the release notes"),
            fixture.columns.last(),
        )
        val open = E2eBackend.createTask(fixture, t("Прогнать регресс", "Run the regression pass"))
        E2eBackend.setTaskMilestone(fixture, done.id, release.id)
        E2eBackend.setTaskMilestone(fixture, open.id, release.id)

        mount {
            MilestonesScreen(
                workspaceId = fixture.workspace.id,
                projects = listOf(fixture.project),
                workspace = fixture.workspace,
                glProjectId = null,
                onOpenMilestone = { _, _ -> },
            )
        }
        compose.awaitText(releaseName)

        compose.shoot("milestones-mobile")
    }

    /**
     * Demo-shaped board content: enough cards, tags and columns that the shot
     * shows a board someone works on rather than an empty grid.
     *
     * Returns the card the task-screen shot opens.
     */
    private fun seedBoardContent(): website.msdnna.tessera.data.model.Task {
        val fixture = e2e.fixture
        val columns = fixture.columns
        val design = E2eBackend.createTag(fixture, t("дизайн", "design"), "#7c5cff")
        val backend = E2eBackend.createTag(fixture, t("бэкенд", "backend"), "#0eb0a9")

        val hero = E2eBackend.createTask(fixture, t("Экран входа: собрать по макету", "Sign-in screen: build from the mockup"), columns[0])
        E2eBackend.addTaskTag(fixture, hero.id, design.id)
        val second = E2eBackend.createTask(fixture, t("Ротация refresh-токена", "Refresh token rotation"), columns[0])
        E2eBackend.addTaskTag(fixture, second.id, backend.id)
        // The phone shows one column at a time, so the first one carries the shot:
        // two cards left it half empty, which reads as «здесь ничего нет».
        val third = E2eBackend.createTask(fixture, t("Оффлайн-режим: план", "Offline mode: the plan"), columns[0])
        E2eBackend.addTaskTag(fixture, third.id, design.id)
        E2eBackend.createTask(fixture, t("Сверить макеты с вебом", "Check the mockups against the web"), columns[0])
        val inProgress = E2eBackend.createTask(
            fixture,
            t("Импорт docx: таблицы", "docx import: tables"),
            columns.getOrElse(1) { columns[0] },
        )
        E2eBackend.addTaskTag(fixture, inProgress.id, backend.id)
        E2eBackend.createTask(
            fixture,
            t("Пуш-уведомления о напоминаниях", "Push notifications for reminders"),
            columns.getOrElse(2) { columns[0] },
        )
        E2eBackend.createTask(fixture, t("Тёмная тема в редакторе", "Dark theme in the editor"), columns.last())
        return hero
    }

    /**
     * Mounts [content] under the theme whose light/dark flag [shoot] flips.
     *
     * `MainScreen` uses `systemBarsPadding`, but in this host it measures zero:
     * the bare test activity draws edge to edge and never gets the insets
     * dispatched, so the padding collapses and the clock and the battery icon
     * land on top of the app's own toolbar. The bars are therefore measured off
     * the window ([measureSystemBars]) and applied as a plain padding — the same
     * gap `MainScreen` would leave on a phone. [crop] then cuts those two strips
     * back off, so the shot is the app alone, the way the web ones are.
     *
     * Hiding the bars instead does not work here: the emulator answers an
     * immersive request with its own «Viewing full screen» card, which lands in
     * the middle of the shot and dims everything under it.
     */
    private fun mount(content: @Composable () -> Unit) {
        val bars = measureSystemBars()
        chrome = bars
        val theme: MutableState<Boolean> = dark
        val language = lang
        compose.setContent {
            val isDark by theme
            // The same wrapper the app itself switches languages with: it swaps
            // LocalResources, so every `stringResource` under it answers in the
            // shot's language. The device locale is left alone deliberately —
            // changing it would restart the emulator's own UI mid-run, and the
            // app's language is a profile setting anyway, not a system one.
            AppLocale(language = language) {
                val focusManager = LocalFocusManager.current
                SideEffect { focus = focusManager }
                val density = LocalDensity.current
                TesseraTheme(isDark = isDark) {
                    Surface(Modifier.fillMaxSize(), color = Tessera.colors.bg) {
                        Box(
                            Modifier.fillMaxSize().padding(
                                top = with(density) { bars.top.toDp() },
                                bottom = with(density) { bars.bottom.toDp() },
                            ),
                        ) { content() }
                    }
                }
            }
        }
    }

    /**
     * Height of the status bar and of the navigation bar, in pixels.
     *
     * Read off the window first; the platform dimens are the fallback for the
     * case the window has not been laid out yet, when the root insets come back
     * empty and a zero padding would put the clock back over the toolbar.
     */
    private fun measureSystemBars(): Insets {
        var bars = Insets.NONE
        compose.activityRule.scenario.onActivity { activity ->
            bars = ViewCompat.getRootWindowInsets(activity.window.decorView)
                ?.getInsets(WindowInsetsCompat.Type.systemBars())
                ?: Insets.NONE
        }
        if (bars.top > 0 || bars.bottom > 0) return bars
        val resources = InstrumentationRegistry.getInstrumentation().targetContext.resources
        return Insets.of(0, resources.barHeight("status_bar_height"), 0, resources.barHeight("navigation_bar_height"))
    }

    private fun Resources.barHeight(name: String): Int {
        val id = getIdentifier(name, "dimen", "android")
        return if (id > 0) getDimensionPixelSize(id) else 0
    }

    /**
     * Drops the field focus, and the on-screen keyboard with it.
     *
     * A scene that types has to. The IME slides up on its own schedule, so the
     * first take of [markdownEditor] came out with the editor visible in one
     * theme and buried under the keyboard in the other — and while the keyboard
     * is up the emulator paints its own «See more features» balloon over the app,
     * which is device chrome the manual should not show.
     */
    private fun dismissKeyboard() {
        compose.runOnUiThread { focus?.clearFocus(force = true) }
        compose.waitForIdle()
    }

    private fun ComposeContentTestRule.awaitText(text: String) {
        waitUntil(AWAIT_MS) { onAllNodesWithText(text, substring = true).fetchSemanticsNodes().isNotEmpty() }
    }

    /**
     * Writes `<name>-light.png` and `<name>-dark.png` (`<name>-<scheme>.<lang>.png`
     * outside Russian). The suffixes are not decoration: `helpAssetUrl` (web and
     * Kotlin alike) finds a dark screenshot by swapping exactly that trailing
     * `-light`, so a shot named otherwise simply never appears in the dark theme.
     */
    private fun ComposeContentTestRule.shoot(name: String) {
        dark.value = false
        waitForIdle()
        write("$name-light$langSuffix.png", screen())
        dark.value = true
        waitForIdle()
        write("$name-dark$langSuffix.png", screen())
    }

    /**
     * The whole device screen, not `onRoot().captureToImage()`.
     *
     * A task opens in a dialog, so that scene has **two** composition roots —
     * the board underneath and the dialog window over it — and `captureToImage`
     * demands exactly one, failing the shot outright. Photographing the screen
     * composites the windows the way the reader sees them, which is what a
     * manual screenshot would have shown anyway.
     *
     * `waitForIdleSync` after Compose's own idle wait: composition being settled
     * says nothing about the frame having reached the surface UiAutomation reads.
     *
     * And it is not enough on its own. An idle message loop still leaves list
     * items mid-entrance and the surface mid-repaint, and the screenshot then
     * comes out torn — two states of the same list superimposed, glyphs clipped.
     * So the shot is taken repeatedly until two consecutive ones are identical:
     * a frame that no longer changes is a frame that has finished animating.
     */
    private fun screen(): Bitmap {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        instrumentation.waitForIdleSync()
        var last = instrumentation.uiAutomation.takeScreenshot()
        repeat(STABLE_TRIES) {
            SystemClock.sleep(STABLE_PAUSE_MS)
            instrumentation.waitForIdleSync()
            val next = instrumentation.uiAutomation.takeScreenshot()
            if (next.sameAs(last)) return crop(next)
            last.recycle()
            last = next
        }
        return crop(last)
    }

    /**
     * Cuts the status and navigation bars off the device screenshot.
     *
     * The scene under them is empty by construction — [mount] pads the content
     * away from both — so this removes the emulator's clock and gesture bar and
     * nothing of Tessera. What is left is a phone-shaped shot of the app, next
     * to the web's browser-shaped ones.
     */
    private fun crop(full: Bitmap): Bitmap {
        val top = chrome.top.coerceIn(0, full.height)
        val height = (full.height - top - chrome.bottom).coerceIn(1, full.height - top)
        if (top == 0 && height == full.height) return full
        return Bitmap.createBitmap(full, 0, top, full.width, height)
    }

    private fun write(fileName: String, bitmap: Bitmap) {
        // The app's own **internal** files dir. The external one looks friendlier
        // (`/sdcard/Android/data/…`) but since Android 11 the shell user cannot
        // read another app's directory there — the run would go green and `adb
        // pull` would then find nothing. `make android-shots` fetches these
        // through `adb root`, which the AVD's userdebug image allows.
        val dir = File(
            InstrumentationRegistry.getInstrumentation().targetContext.filesDir,
            "help-shots",
        )
        dir.mkdirs()
        File(dir, fileName).outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
    }

    private companion object {
        const val AWAIT_MS = 20_000L

        /** The language whose shots carry the bare file name. */
        const val DEFAULT_LANG = "ru"

        /** Read from the companion so instance properties can use it in their own
         *  initialisers, whatever order they end up in. */
        fun shotsLang(): String =
            InstrumentationRegistry.getArguments().getString("shotsLang")?.takeIf { it.isNotBlank() } ?: DEFAULT_LANG

        /** How many times [screen] re-shoots while the frame keeps changing. */
        const val STABLE_TRIES = 12

        /** Long enough for a Compose enter animation to move visibly between shots. */
        const val STABLE_PAUSE_MS = 250L
    }
}
