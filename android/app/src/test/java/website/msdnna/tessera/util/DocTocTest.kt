package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import com.google.gson.JsonParser
import org.junit.Test

/**
 * The reader's outline (#2733, §3 of #2894). Mirrors `ut-docToc.spec.js`: the
 * two clients must agree on the structure they show, because the anchors are the
 * same block ids and a reader who used both would otherwise see two documents.
 */
class DocTocTest {
    private fun outline(json: String) = docOutline(parseDocBlocks(JsonParser.parseString(json)))

    @Test
    fun `headings are collected in reading order with their level`() {
        val rows = outline(
            """
            {"type":"doc","content":[
              {"type":"heading","attrs":{"id":"h1","level":1},"content":[{"type":"text","text":"Введение"}]},
              {"type":"paragraph","attrs":{"id":"p1"},"content":[{"type":"text","text":"текст"}]},
              {"type":"heading","attrs":{"id":"h2","level":2},"content":[{"type":"text","text":"Часть"}]}
            ]}
            """.trimIndent(),
        )
        assertThat(rows.map { it.id }).containsExactly("h1", "h2").inOrder()
        assertThat(rows.map { it.text }).containsExactly("Введение", "Часть").inOrder()
        assertThat(rows.map { it.level }).containsExactly(1, 2).inOrder()
    }

    @Test
    fun `nesting is relative, not the heading number`() {
        // A document that starts at h2 and drops to h4 would otherwise be drawn
        // with empty indent steps in front of every entry.
        val rows = outline(
            """
            {"type":"doc","content":[
              {"type":"heading","attrs":{"id":"a","level":2},"content":[{"type":"text","text":"A"}]},
              {"type":"heading","attrs":{"id":"b","level":4},"content":[{"type":"text","text":"B"}]},
              {"type":"heading","attrs":{"id":"c","level":4},"content":[{"type":"text","text":"C"}]},
              {"type":"heading","attrs":{"id":"d","level":3},"content":[{"type":"text","text":"D"}]},
              {"type":"heading","attrs":{"id":"e","level":1},"content":[{"type":"text","text":"E"}]}
            ]}
            """.trimIndent(),
        )
        assertThat(rows.map { it.depth }).containsExactly(0, 1, 1, 1, 0).inOrder()
    }

    @Test
    fun `an empty heading keeps its row`() {
        // A heading is created empty and typed into; dropping it until the first
        // character would make the list jump under the reader's finger.
        val rows = outline(
            """{"type":"doc","content":[{"type":"heading","attrs":{"id":"h","level":1}}]}""",
        )
        assertThat(rows.single().id).isEqualTo("h")
        assertThat(rows.single().text).isEmpty()
    }

    @Test
    fun `a document without headings has no outline`() {
        assertThat(outline("""{"type":"doc","content":[{"type":"paragraph"}]}""")).isEmpty()
        assertThat(docOutline(emptyList())).isEmpty()
    }

    @Test
    fun `a row points at the block the reader will scroll to`() {
        val blocks = parseDocBlocks(
            JsonParser.parseString(
                """
                {"type":"doc","content":[
                  {"type":"paragraph","attrs":{"id":"p"},"content":[{"type":"text","text":"до"}]},
                  {"type":"heading","attrs":{"id":"h","level":1},"content":[{"type":"text","text":"Раздел"}]}
                ]}
                """.trimIndent(),
            ),
        )
        val row = docOutline(blocks).single()
        assertThat(docBlockIndex(blocks, row.id)).isEqualTo(1)
        // A heading that is no longer in the body must report «nowhere» rather
        // than scroll the reader to the top of the document.
        assertThat(docBlockIndex(blocks, "gone")).isEqualTo(-1)
    }
}
