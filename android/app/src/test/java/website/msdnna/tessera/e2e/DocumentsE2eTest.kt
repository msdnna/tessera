package website.msdnna.tessera.e2e

import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import com.google.common.truth.Truth.assertThat
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
        compose.pickDocMenu(TestTags.DOCUMENT_ACTION_CHILDREN)

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
        compose.pickDocMenu(TestTags.DOCUMENT_BACK)
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
        compose.pickDocMenu(TestTags.DOCUMENT_ACTION_RENAME)
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
        compose.pickDocMenu(TestTags.DOCUMENT_ACTION_REMOVE)
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
        // On the tag, not on the sentence: this tier runs under Robolectric's
        // default locale, so the interface is the English one and an assertion
        // on the Russian copy fails on the language rather than on the state.
        compose.awaitTag(TestTags.DOCUMENT_READER_EMPTY)
    }

    @Test
    fun `a remark typed in the panel lands on the server and the badge counts it`() {
        val fixture = e2e.fixture
        val doc = E2eBackend.createDocument(fixture, "Обсуждаемый ${System.nanoTime()}")
        E2eBackend.setDocumentContent(fixture, doc, BODY)
        val seeded = E2eBackend.createDocumentComment(
            fixture,
            doc.id,
            body = "Уточните формулировку",
            blockId = "p1",
            quote = "обычный и жирный",
        )

        compose.setDocumentsContent(fixture, doc.id)
        compose.onNodeWithTag(TestTags.documentRow(doc.id)).performClick()
        compose.awaitTag(TestTags.DOCUMENT_READER)

        // On a phone the panel is *over* the page, so the badge is the only thing
        // that says a remark exists at all — an unread discussion behind a button
        // with no number on it is an unread discussion nobody opens.
        compose.awaitTextOn(TestTags.DOCUMENT_COMMENTS_BADGE, "1")

        compose.pickDocMenu(TestTags.DOCUMENT_COMMENTS_OPEN)
        compose.awaitTag(TestTags.DOCUMENT_COMMENTS)
        // Scoped to the seeded thread's own card: the body would also match a
        // panel that dropped the threading and rendered one flat list.
        compose.awaitTextIn(TestTags.documentCommentThread(seeded.id), "Уточните формулировку")

        val added = "Поправил ${System.nanoTime()}"
        compose.onNodeWithTag(TestTags.DOCUMENT_COMMENT_DRAFT).performTextInput(added)
        compose.onNodeWithTag(TestTags.DOCUMENT_COMMENT_SEND).performClick()

        // Read back from Postgres rather than from the panel: a row painted
        // optimistically satisfies an on-screen assertion even when the POST
        // never landed.
        val stored = compose.awaitServer("the remark typed into ${doc.id}") {
            E2eBackend.documentComments(fixture, doc.id).firstOrNull { it.body == added }
        }
        // Opened from the bar, so it hangs on the document rather than on
        // whichever block happened to be under the panel.
        assertThat(stored.blockId).isEmpty()
    }

    @Test
    fun `resolving a thread settles it on the server, not only on screen`() {
        val fixture = e2e.fixture
        val doc = E2eBackend.createDocument(fixture, "Решаемый ${System.nanoTime()}")
        E2eBackend.setDocumentContent(fixture, doc, BODY)
        val seeded = E2eBackend.createDocumentComment(
            fixture,
            doc.id,
            body = "Здесь всё верно",
            blockId = "p1",
        )

        compose.setDocumentsContent(fixture, doc.id)
        compose.onNodeWithTag(TestTags.documentRow(doc.id)).performClick()
        compose.awaitTag(TestTags.DOCUMENT_READER)
        compose.pickDocMenu(TestTags.DOCUMENT_COMMENTS_OPEN)
        compose.awaitTag(TestTags.DOCUMENT_COMMENTS)

        compose.onNodeWithTag(TestTags.documentCommentResolve(seeded.id)).performClick()
        compose.awaitTag(TestTags.documentCommentResolved(seeded.id))

        compose.awaitServer("the resolve of ${seeded.id}") {
            E2eBackend.documentComments(fixture, doc.id).firstOrNull { it.id == seeded.id && it.isResolved }
        }
    }

    @Test
    fun `restoring a version puts the old text back into the document`() {
        val fixture = e2e.fixture
        val doc = E2eBackend.createDocument(fixture, "Откатываемый ${System.nanoTime()}")

        // Three writes rather than two, and the middle one is a manual snapshot
        // on purpose: consecutive saves by the same author extend the newest
        // journal entry instead of adding one, so without it the journal would
        // hold a single row and there would be nothing to compare or roll back to.
        E2eBackend.setDocumentContent(fixture, doc, body(AGREED))
        val snapshot = E2eBackend.snapshotDocument(fixture, doc.id, "Согласованная")
        E2eBackend.setDocumentContent(fixture, E2eBackend.document(fixture, doc.id), body(DRAFT))

        compose.setDocumentsContent(fixture, doc.id)
        compose.onNodeWithTag(TestTags.documentRow(doc.id)).performClick()
        compose.awaitTag(TestTags.DOCUMENT_READER)
        compose.awaitText(DRAFT)

        compose.pickDocMenu(TestTags.DOCUMENT_HISTORY_OPEN)
        compose.awaitTag(TestTags.DOCUMENT_HISTORY)
        compose.onNodeWithTag(TestTags.documentVersionRow(snapshot.id)).performClick()

        // The comparison is against the document as it stands, and the body of a
        // version arrives only when it is picked — so this also proves the
        // journal fetched the entry rather than diffing the preview line.
        compose.awaitTag(TestTags.DOCUMENT_HISTORY_SUMMARY)
        compose.awaitTextIn(TestTags.DOCUMENT_HISTORY_DIFF, AGREED)

        compose.onNodeWithTag(TestTags.DOCUMENT_RESTORE).performClick()
        compose.awaitTag(TestTags.DOCUMENT_RESTORE_CONFIRM)
        compose.onNodeWithTag(TestTags.DOCUMENT_RESTORE_CONFIRM).performClick()

        // The picked entry is no longer the comparison: the rollback made it the
        // current state, and a panel still offering «restore» would be offering
        // to restore what is already there.
        compose.awaitNoTag(TestTags.DOCUMENT_HISTORY_DIFF)
        compose.onNodeWithTag(TestTags.DOCUMENT_HISTORY_CLOSE).performClick()
        compose.awaitNoTag(TestTags.DOCUMENT_HISTORY)

        // The reader is showing the restored body — and so is Postgres. Either
        // assertion alone passes on a bug the other one catches: the screen can
        // be repainted from a response that was never written, and the row can
        // be written under a reader still holding the replaced text.
        compose.awaitText(AGREED)
        val rolled = compose.awaitServer("the rollback of ${doc.id}") {
            E2eBackend.document(fixture, doc.id).takeIf { AGREED in it.content?.toString().orEmpty() }
        }
        // Replaced, not merged: a rollback that appended the old blocks instead
        // of taking the new ones away satisfies the assertion above and leaves
        // on the page exactly the text it was asked to undo.
        assertThat(rolled.content?.toString().orEmpty()).doesNotContain(DRAFT)
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

        /**
         * The two states the rollback spec moves a document between.
         *
         * Distinct words rather than «версия 1»/«версия 2»: the assertions are
         * about which text is on screen, and near-identical fixtures make a
         * failure unreadable.
         */
        const val AGREED = "Согласованная формулировка"
        const val DRAFT = "Черновая правка"

        /** A one-paragraph body — the diff panel is a lazy list, and a fixture
         *  taller than the panel would put the row being asserted below the fold. */
        fun body(text: String) = mapOf(
            "type" to "doc",
            "content" to listOf(
                mapOf(
                    "type" to "paragraph",
                    "attrs" to mapOf("id" to "p1"),
                    "content" to listOf(mapOf("type" to "text", "text" to text)),
                ),
            ),
        )

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
