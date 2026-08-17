package website.msdnna.tessera.e2e

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
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
@RunWith(RobolectricTestRunner::class)
class DocumentsE2eTest {
    private val e2e = E2eRule()
    private val compose = createComposeRule()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    @Test
    fun `the tree lists documents and their children`() {
        val fixture = e2e.fixture
        val parent = E2eBackend.createDocument(fixture, "Регламент ${System.nanoTime()}")
        val child = E2eBackend.createDocument(fixture, "Приложение А", parentId = parent.id)

        compose.setDocumentsContent(fixture, parent.id)

        // Both ends of the nesting: the child is a row of its own, not folded
        // into its parent and not dropped for having one.
        compose.awaitTag(TestTags.documentRow(child.id))
        compose.onNodeWithTag(TestTags.documentRow(parent.id)).assertIsDisplayed()
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
