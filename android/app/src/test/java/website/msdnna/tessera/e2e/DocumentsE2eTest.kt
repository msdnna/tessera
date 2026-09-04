package website.msdnna.tessera.e2e

import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import org.junit.Ignore
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.ui.TestTags

/**
 * The documents section end to end (#2735): a real backend, a real ProseMirror
 * body written by the server's own validator, and the app's real ViewModel and
 * Retrofit stack underneath.
 *
 * This is the tier the unit tests cannot reach. `DocBlocksTest` proves the
 * parser turns a tree into rows, but it feeds itself its own JSON — it would
 * keep passing if `Document.content` were misnamed and always arrived null, or
 * if the list endpoint were called with the wrong path. Here the bytes come
 * from Postgres.
 */
// The grid, its menu and the dialogs are taller than Robolectric's default
// 320x470 screen, where a tap below the fold is swallowed without a word.
@Config(qualifiers = "w400dp-h900dp")
@RunWith(RobolectricTestRunner::class)
class DocumentsE2eTest {
    private val e2e = E2eRule()
    private val compose = createComposeRule()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    @Test
    fun `the grid shows one level and the trail walks into the nested one`() {
        val fixture = e2e.fixture
        val parent = E2eBackend.createDocument(fixture, "Регламент ${System.nanoTime()}")
        val child = E2eBackend.createDocument(fixture, "Приложение А", parentId = parent.id)

        compose.setDocumentsContent(fixture, parent.id)

        // The root level is exactly the root documents: a child shown here would
        // be the flat list the grid replaced (#2726).
        compose.onNodeWithTag(TestTags.documentRow(child.id)).assertDoesNotExist()

        // Nesting is walked from the reader, as on the web.
        compose.onNodeWithTag(TestTags.documentRow(parent.id)).performClick()
        compose.awaitTag(TestTags.DOCUMENT_READER)
        compose.onNodeWithTag(TestTags.DOCUMENT_ACTIONS).performClick()
        compose.awaitTag(TestTags.DOCUMENT_ACTION_CHILDREN)
        compose.onNodeWithTag(TestTags.DOCUMENT_ACTION_CHILDREN).performClick()

        // Inside the container: the child, and only the child.
        compose.awaitTag(TestTags.documentRow(child.id))
        compose.onNodeWithTag(TestTags.documentRow(parent.id)).assertDoesNotExist()

        // And the trail is the way back out.
        compose.onNodeWithTag(TestTags.DOCUMENTS_CRUMB_ROOT).performClick()
        compose.awaitTag(TestTags.documentRow(parent.id))
    }

    @Ignore(DIALOG_TEXT_FIELD_HANGS)
    @Test
    fun `a document created from the grid opens and stays in the list`() {
        val fixture = e2e.fixture
        val anchor = E2eBackend.createDocument(fixture, "Якорь ${System.nanoTime()}")
        val title = "Созданный ${System.nanoTime()}"

        compose.setDocumentsContent(fixture, anchor.id)
        compose.onNodeWithTag(TestTags.DOCUMENTS_CREATE).performClick()
        compose.awaitTag(TestTags.DOCUMENT_TITLE_INPUT)
        compose.onNodeWithTag(TestTags.DOCUMENT_TITLE_INPUT).performTextClearance()
        compose.onNodeWithTag(TestTags.DOCUMENT_TITLE_INPUT).performTextInput(title)
        compose.onNodeWithTag(TestTags.DOCUMENT_TITLE_CONFIRM).performClick()

        // Created *and* opened, the way the web does it — the reader carries the
        // title the dialog was given, so this is the new document and not the
        // one the grid happened to be showing.
        compose.awaitTag(TestTags.DOCUMENT_READER)
        compose.awaitTextIn(TestTags.DOCUMENT_READER, title)

        // Back on the grid it is a tile of its own: the list was reloaded, not
        // just the reader pushed over a stale one.
        compose.onNodeWithTag(TestTags.DOCUMENT_BACK).performClick()
        compose.awaitNoTag(TestTags.DOCUMENT_READER)
        compose.awaitText(title)
    }

    @Ignore(DIALOG_TEXT_FIELD_HANGS)
    @Test
    fun `renaming a document renames its tile`() {
        val fixture = e2e.fixture
        val doc = E2eBackend.createDocument(fixture, "Старое имя ${System.nanoTime()}")
        val renamed = "Новое имя ${System.nanoTime()}"

        compose.setDocumentsContent(fixture, doc.id)
        compose.onNodeWithTag(TestTags.documentRow(doc.id)).performClick()
        compose.awaitTag(TestTags.DOCUMENT_READER)
        compose.onNodeWithTag(TestTags.DOCUMENT_ACTIONS).performClick()
        compose.awaitTag(TestTags.DOCUMENT_ACTION_RENAME)
        compose.onNodeWithTag(TestTags.DOCUMENT_ACTION_RENAME).performClick()
        compose.awaitTag(TestTags.DOCUMENT_TITLE_INPUT)
        compose.onNodeWithTag(TestTags.DOCUMENT_TITLE_INPUT).performTextClearance()
        compose.onNodeWithTag(TestTags.DOCUMENT_TITLE_INPUT).performTextInput(renamed)
        compose.onNodeWithTag(TestTags.DOCUMENT_TITLE_CONFIRM).performClick()

        // The reader is renamed, and so is the tile behind it: the PATCH went to
        // the server and the list was re-read, rather than only the open copy.
        compose.awaitTextIn(TestTags.DOCUMENT_READER, renamed)
        compose.awaitTextOn(TestTags.documentRow(doc.id), renamed)
    }

    @Test
    fun `deleting a container takes its nested documents with it`() {
        // The endpoint refuses a document that has children unless the delete is
        // recursive, so this is also the guard on the confirmation actually
        // sending `?recursive=true` — the wording alone would still 409.
        val fixture = e2e.fixture
        val parent = E2eBackend.createDocument(fixture, "Удаляемый ${System.nanoTime()}")
        E2eBackend.createDocument(fixture, "Вложенный", parentId = parent.id)

        compose.setDocumentsContent(fixture, parent.id)
        compose.onNodeWithTag(TestTags.documentRow(parent.id)).performClick()
        compose.awaitTag(TestTags.DOCUMENT_READER)
        compose.onNodeWithTag(TestTags.DOCUMENT_ACTIONS).performClick()
        compose.awaitTag(TestTags.DOCUMENT_ACTION_REMOVE)
        compose.onNodeWithTag(TestTags.DOCUMENT_ACTION_REMOVE).performClick()
        compose.awaitTag(TestTags.DOCUMENT_REMOVE_CONFIRM)
        compose.onNodeWithTag(TestTags.DOCUMENT_REMOVE_CONFIRM).performClick()

        compose.awaitNoTag(TestTags.DOCUMENT_READER)
        compose.awaitNoTag(TestTags.documentRow(parent.id))
    }

    @Test
    fun `opening a document renders its blocks`() {
        val fixture = e2e.fixture
        val doc = E2eBackend.createDocument(fixture, "Тело ${System.nanoTime()}")
        E2eBackend.setDocumentContent(fixture, doc, BODY)

        compose.setDocumentsContent(fixture, doc.id)
        compose.onNodeWithTag(TestTags.documentRow(doc.id)).performClick()
        compose.awaitTag(TestTags.DOCUMENT_READER)

        // Asserting on the body's own words, not on UI copy: this text is
        // fixture data, and it is exactly what «the document rendered» means.
        // One assertion per block kind that has its own renderer, so a broken
        // branch cannot hide behind a working one.
        compose.awaitText("Заголовок раздела")
        compose.awaitText("жирный")
        compose.awaitText("Первый пункт")
        compose.awaitText("Вложенный")
        compose.awaitText("Сделано")
        compose.awaitText("Цитата")
        compose.awaitText("Колонка")
    }

    @Test
    fun `an empty document opens without content rather than staying blank`() {
        // A document created but never written to has `{"type":"doc","content":[]}`
        // as its body. The reader must say so, not sit on a spinner — the state
        // is indistinguishable from «still loading» from the outside.
        val fixture = e2e.fixture
        val doc = E2eBackend.createDocument(fixture, "Пустой ${System.nanoTime()}")

        compose.setDocumentsContent(fixture, doc.id)
        compose.onNodeWithTag(TestTags.documentRow(doc.id)).performClick()

        compose.awaitTag(TestTags.DOCUMENT_READER)
        compose.awaitText("Документ пуст")
    }

    /** Waits for a node carrying [text] anywhere in the tree. */
    private fun androidx.compose.ui.test.junit4.ComposeContentTestRule.awaitText(text: String) {
        waitUntil(TIMEOUT_MS) {
            onAllNodes(hasText(text, substring = true)).fetchSemanticsNodes().isNotEmpty()
        }
    }

    private companion object {
        /**
         * Why the two dialog-driven specs above do not run in this tier.
         *
         * A Compose text field inside an `androidx.compose.ui.window.Dialog`
         * never lets Robolectric's main looper go idle, so every wait after the
         * dialog opens dies with `AppNotIdleException`. Measured, not guessed —
         * a stock `BasicTextField` in a stock `Dialog`, with no Tessera code in
         * the composition at all, hangs exactly the same way, while the same
         * field outside a dialog settles at once. Freezing the clock
         * (`mainClock.autoAdvance = false`) does not help either, so it is the
         * dialog's own window rather than an animation.
         *
         * So create and rename have no automated coverage on this tier yet;
         * they belong on the instrumented one (`make test-android-instrumented`),
         * which runs on a real device where this does not happen. Deleting still
         * runs here — its confirmation has no text field — and so does every
         * spec that only walks the grid.
         */
        const val DIALOG_TEXT_FIELD_HANGS =
            "Compose text field inside a Dialog never idles under Robolectric (see the note in this file)"

        const val TIMEOUT_MS = 10_000L

        /** One block of every kind the reader renders. */
        val BODY = mapOf(
            "type" to "doc",
            "content" to listOf(
                mapOf(
                    "type" to "heading",
                    "attrs" to mapOf("id" to "h1", "level" to 1),
                    "content" to listOf(mapOf("type" to "text", "text" to "Заголовок раздела")),
                ),
                mapOf(
                    "type" to "paragraph",
                    "attrs" to mapOf("id" to "p1"),
                    "content" to listOf(
                        mapOf("type" to "text", "text" to "обычный и "),
                        mapOf(
                            "type" to "text",
                            "text" to "жирный",
                            "marks" to listOf(mapOf("type" to "bold")),
                        ),
                    ),
                ),
                mapOf(
                    "type" to "bulletList",
                    "attrs" to mapOf("id" to "ul"),
                    "content" to listOf(
                        mapOf(
                            "type" to "listItem",
                            "content" to listOf(
                                para("Первый пункт"),
                                mapOf(
                                    "type" to "bulletList",
                                    "content" to listOf(
                                        mapOf("type" to "listItem", "content" to listOf(para("Вложенный"))),
                                    ),
                                ),
                            ),
                        ),
                    ),
                ),
                mapOf(
                    "type" to "taskList",
                    "attrs" to mapOf("id" to "tl"),
                    "content" to listOf(
                        mapOf(
                            "type" to "taskItem",
                            "attrs" to mapOf("checked" to true),
                            "content" to listOf(para("Сделано")),
                        ),
                    ),
                ),
                mapOf(
                    "type" to "blockquote",
                    "attrs" to mapOf("id" to "bq"),
                    "content" to listOf(para("Цитата")),
                ),
                mapOf(
                    "type" to "codeBlock",
                    "attrs" to mapOf("id" to "cb", "language" to "kotlin"),
                    "content" to listOf(mapOf("type" to "text", "text" to "fun main() = Unit")),
                ),
                mapOf("type" to "horizontalRule", "attrs" to mapOf("id" to "hr")),
                mapOf(
                    "type" to "table",
                    "attrs" to mapOf("id" to "tb"),
                    "content" to listOf(
                        mapOf(
                            "type" to "tableRow",
                            "content" to listOf(
                                mapOf("type" to "tableHeader", "content" to listOf(para("Колонка"))),
                            ),
                        ),
                        mapOf(
                            "type" to "tableRow",
                            "content" to listOf(
                                mapOf("type" to "tableCell", "content" to listOf(para("Значение"))),
                            ),
                        ),
                    ),
                ),
            ),
        )

        fun para(text: String) = mapOf(
            "type" to "paragraph",
            "content" to listOf(mapOf("type" to "text", "text" to text)),
        )
    }
}
