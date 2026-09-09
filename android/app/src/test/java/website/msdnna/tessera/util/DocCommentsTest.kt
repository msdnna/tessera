package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import com.google.gson.JsonParser
import org.junit.Test
import website.msdnna.tessera.data.model.DocumentComment

/**
 * Threading and anchoring of document annotations (#2730, §5 of #2894).
 *
 * These rules are where a discussion silently goes wrong: a thread whose block
 * was rewritten must still be shown (it is usually the remark that asked for the
 * rewrite), a settled one must stop counting, and the order the cards come out
 * in is what the sheet reads top to bottom. None of that needs a screen, so none
 * of it is tested through one.
 */
class DocCommentsTest {
    private fun comment(
        id: String,
        body: String = "b",
        blockId: String = "",
        parentId: String? = null,
        created: String = "2026-09-01T10:00:00Z",
        resolvedAt: String? = null,
        authorName: String? = null,
        authorEmail: String? = null,
    ) = DocumentComment(
        id = id,
        blockId = blockId,
        parentId = parentId,
        body = body,
        createdAt = created,
        resolvedAt = resolvedAt,
        authorName = authorName,
        authorEmail = authorEmail,
    )

    @Test
    fun `replies hang off their root, in the order the server sent them`() {
        val threads = buildDocThreads(
            listOf(
                comment("r1", blockId = "b1"),
                comment("a1", parentId = "r1"),
                comment("a2", parentId = "r1"),
                comment("r2"),
            ),
        )
        assertThat(threads.map { it.id }).containsExactly("r1", "r2").inOrder()
        assertThat(threads[0].replies.map { it.id }).containsExactly("a1", "a2").inOrder()
        assertThat(threads[1].replies).isEmpty()
    }

    @Test
    fun `a reply whose root is missing is dropped, not promoted`() {
        // The root carries the anchor and the resolved state, so a lone reply
        // would render as an answer to a question nobody can see.
        val threads = buildDocThreads(listOf(comment("orphan", parentId = "gone")))
        assertThat(threads).isEmpty()
    }

    @Test
    fun `open threads come first, newest first inside each group`() {
        val sorted = sortDocThreads(
            listOf(
                DocThread(comment("old", created = "2026-09-01T10:00:00Z")),
                DocThread(comment("done", created = "2026-09-03T10:00:00Z", resolvedAt = "2026-09-04T10:00:00Z")),
                DocThread(comment("new", created = "2026-09-02T10:00:00Z")),
            ),
        )
        assertThat(sorted.map { it.id }).containsExactly("new", "old", "done").inOrder()
    }

    @Test
    fun `a thread whose block was deleted is kept apart, never dropped`() {
        val groups = splitDocThreads(
            listOf(
                DocThread(comment("live", blockId = "b1")),
                DocThread(comment("gone", blockId = "b9")),
                DocThread(comment("doc")),
            ),
            listOf("b1", "b2"),
        )
        assertThat(groups.anchored.map { it.id }).containsExactly("live")
        assertThat(groups.detached.map { it.id }).containsExactly("gone")
        assertThat(groups.document.map { it.id }).containsExactly("doc")
        assertThat(groups.isEmpty).isFalse()
    }

    @Test
    fun `anchored threads come out in document order, not in list order`() {
        // The sheet is read top to bottom against the text: a remark on the last
        // paragraph must not sit above one on the first just because it is newer.
        val groups = splitDocThreads(
            listOf(
                DocThread(comment("third", blockId = "c")),
                DocThread(comment("first", blockId = "a")),
                DocThread(comment("second", blockId = "b")),
            ),
            listOf("a", "b", "c"),
        )
        assertThat(groups.anchored.map { it.id }).containsExactly("first", "second", "third").inOrder()
    }

    @Test
    fun `two threads on one block keep the order they arrived in`() {
        val groups = splitDocThreads(
            listOf(
                DocThread(comment("newer", blockId = "a")),
                DocThread(comment("older", blockId = "a")),
            ),
            listOf("a"),
        )
        assertThat(groups.anchored.map { it.id }).containsExactly("newer", "older").inOrder()
    }

    @Test
    fun `the count is of open discussions only`() {
        val threads = listOf(
            DocThread(comment("open1", blockId = "a")),
            DocThread(comment("open2", blockId = "b")),
            DocThread(comment("settled", blockId = "a", resolvedAt = "2026-09-04T10:00:00Z")),
        )
        assertThat(docOpenThreadCount(threads)).isEqualTo(2)
        assertThat(docOpenThreadCount(threads, "a")).isEqualTo(1)
    }

    @Test
    fun `an author with no name is their email's local part, never a blank row`() {
        assertThat(docCommentAuthor(comment("c", authorName = "Иван"), "Участник")).isEqualTo("Иван")
        assertThat(docCommentAuthor(comment("c", authorEmail = "ivan@example.com"), "Участник"))
            .isEqualTo("ivan")
        assertThat(docCommentAuthor(comment("c"), "Участник")).isEqualTo("Участник")
        // A name of spaces is not a name.
        assertThat(docCommentAuthor(comment("c", authorName = "   "), "Участник")).isEqualTo("Участник")
    }

    @Test
    fun `block ids are collected in reading order, nested ones included`() {
        val json = JsonParser.parseString(
            """
            {"type":"doc","content":[
              {"type":"paragraph","attrs":{"id":"p1"},"content":[{"type":"text","text":"раз"}]},
              {"type":"table","attrs":{"id":"t1"},"content":[
                {"type":"tableRow","content":[
                  {"type":"tableCell","content":[{"type":"paragraph","attrs":{"id":"p2"},"content":[]}]}
                ]}
              ]},
              {"type":"paragraph","content":[{"type":"text","text":"без id"}]},
              {"type":"paragraph","attrs":{"id":"p3"},"content":[]}
            ]}
            """.trimIndent(),
        )
        assertThat(docBlockIdsInOrder(json)).containsExactly("p1", "t1", "p2", "p3").inOrder()
    }

    @Test
    fun `a body with no ids anchors nothing rather than inventing keys`() {
        // The reader invents an id for a node that has none so the list has a
        // key; anchoring must not, or a detached thread would look anchored.
        val json = JsonParser.parseString("""{"type":"doc","content":[{"type":"paragraph"}]}""")
        assertThat(docBlockIdsInOrder(json)).isEmpty()
        assertThat(docBlockIdsInOrder(null)).isEmpty()
    }
}
