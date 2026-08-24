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
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.junit4.ComposeContentTestRule
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
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
import website.msdnna.tessera.e2e.awaitTag
import website.msdnna.tessera.e2e.selectGrouping
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
    private val e2e = E2eRule()
    private val compose = createAndroidComposeRule<ComponentActivity>()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    private val dark = mutableStateOf(false)

    /** System bar heights of the current mount — the strips [crop] cuts away. */
    private var chrome: Insets = Insets.NONE

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
    fun taskScreen() {
        val fixture = e2e.fixture
        val card = seedBoardContent()

        mount { BoardScreen(board = fixture.board, workspaceId = fixture.workspace.id) }
        compose.awaitTag(TestTags.boardColumn(fixture.firstColumn.id))
        compose.onNodeWithTag(TestTags.taskCard(card.id)).performClick()
        compose.awaitTag(TestTags.TASK_TITLE)

        compose.shoot("task-modal-mobile")
    }

    @Test
    fun documents() {
        val fixture = e2e.fixture
        val spec = E2eBackend.createDocument(fixture, "Требования к релизу")
        E2eBackend.createDocument(fixture, "Протокол встречи")
        E2eBackend.createDocument(fixture, "Инструкция для новичка", parentId = spec.id)

        mount { DocumentsScreen(workspaceId = fixture.workspace.id) }
        compose.awaitTag(TestTags.documentRow(spec.id))

        compose.shoot("documents-mobile")
    }

    @Test
    fun notes() {
        val fixture = e2e.fixture
        E2eBackend.createNote(fixture, "Итоги созвона", "— решили выкатывать в пятницу\n— Аня готовит миграцию")
        E2eBackend.createNote(fixture, "Вопросы к заказчику")
        E2eBackend.createNote(fixture, "Черновик анонса")

        mount {
            NotesScreen(
                workspaceId = fixture.workspace.id,
                preselectNoteId = null,
                onPreselectConsumed = {},
            )
        }
        compose.awaitText("Итоги созвона")

        compose.shoot("notes-mobile")
    }

    @Test
    fun reminders() {
        val fixture = e2e.fixture
        E2eBackend.createReminder(fixture, "Позвонить в банк", "2026-09-01T10:00:00Z")
        E2eBackend.createReminder(fixture, "Отправить отчёт", "2026-09-02T09:30:00Z")

        mount { RemindersScreen() }
        compose.awaitText("Позвонить в банк")

        compose.shoot("reminders-mobile")
    }

    @Test
    fun milestones() {
        val fixture = e2e.fixture
        val release = E2eBackend.createMilestone(
            fixture,
            "Релиз 1.0",
            startDate = "2026-08-01T00:00:00Z",
            dueDate = "2026-09-15T00:00:00Z",
        )
        E2eBackend.createMilestone(fixture, "Демо заказчику", dueDate = "2026-10-01T00:00:00Z")
        // A milestone with no tasks shows «нет задач» instead of a progress bar,
        // so one of the two gets tasks — the shot should show both states.
        val done = E2eBackend.createTask(fixture, "Собрать релизные заметки", fixture.columns.last())
        val open = E2eBackend.createTask(fixture, "Прогнать регресс")
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
        compose.awaitText("Релиз 1.0")

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
        val design = E2eBackend.createTag(fixture, "дизайн", "#7c5cff")
        val backend = E2eBackend.createTag(fixture, "бэкенд", "#0eb0a9")

        val hero = E2eBackend.createTask(fixture, "Экран входа: собрать по макету", columns[0])
        E2eBackend.addTaskTag(fixture, hero.id, design.id)
        val second = E2eBackend.createTask(fixture, "Ротация refresh-токена", columns[0])
        E2eBackend.addTaskTag(fixture, second.id, backend.id)
        // The phone shows one column at a time, so the first one carries the shot:
        // two cards left it half empty, which reads as «здесь ничего нет».
        val third = E2eBackend.createTask(fixture, "Оффлайн-режим: план", columns[0])
        E2eBackend.addTaskTag(fixture, third.id, design.id)
        E2eBackend.createTask(fixture, "Сверить макеты с вебом", columns[0])
        val inProgress = E2eBackend.createTask(fixture, "Импорт docx: таблицы", columns.getOrElse(1) { columns[0] })
        E2eBackend.addTaskTag(fixture, inProgress.id, backend.id)
        E2eBackend.createTask(fixture, "Пуш-уведомления о напоминаниях", columns.getOrElse(2) { columns[0] })
        E2eBackend.createTask(fixture, "Тёмная тема в редакторе", columns.last())
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
        compose.setContent {
            val isDark by theme
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

    private fun ComposeContentTestRule.awaitText(text: String) {
        waitUntil(AWAIT_MS) { onAllNodesWithText(text, substring = true).fetchSemanticsNodes().isNotEmpty() }
    }

    /**
     * Writes `<name>-light.png` and `<name>-dark.png`. The suffixes are not
     * decoration: `helpAssetUrl` (web and Kotlin alike) finds a dark screenshot
     * by swapping exactly that trailing `-light`, so a shot named otherwise
     * simply never appears in the dark theme.
     */
    private fun ComposeContentTestRule.shoot(name: String) {
        dark.value = false
        waitForIdle()
        write("$name-light.png", screen())
        dark.value = true
        waitForIdle()
        write("$name-dark.png", screen())
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

        /** How many times [screen] re-shoots while the frame keeps changing. */
        const val STABLE_TRIES = 12

        /** Long enough for a Compose enter animation to move visibly between shots. */
        const val STABLE_PAUSE_MS = 250L
    }
}
